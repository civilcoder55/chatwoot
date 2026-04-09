class Sip::CallService
  pattr_initialize [:sip_call!, :agent!]

  def self.initiate(conversation:, agent:, sdp_offer:)
    channel = conversation.inbox.channel
    raise Sip::CallErrors::CallFailed, 'Not a SIP inbox' unless channel.is_a?(Channel::Sip)

    contact_phone = conversation.contact&.phone_number
    raise Sip::CallErrors::CallFailed, 'Contact phone number not available' if contact_phone.blank?

    result = Sip::GatewayClient.new(channel).initiate_call(from: channel.phone_number, to: contact_phone, sdp_offer: sdp_offer)

    ActiveRecord::Base.transaction do
      sip_call = conversation.account.sip_calls.create!(
        inbox: conversation.inbox,
        conversation: conversation,
        call_id: result['call_id'],
        direction: :outbound,
        status: :ringing,
        meta: { 'sdp_offer' => sdp_offer, 'sdp_answer' => result['sdp_answer'] }
      )

      message = Sip::CallMessageBuilder.create!(conversation: conversation, sip_call: sip_call, user: agent)
      sip_call.update!(message_id: message.id)
      sip_call
    end
  end

  def accept(sdp_answer)
    sip_call.with_lock do
      ensure_ringing!
      ensure_not_already_taken!

      gateway_client.accept_call(sip_call.call_id, sdp_answer)
      sip_call.update!(status: :accepted, accepted_by_agent_id: agent.id)
    end

    Sip::CallMessageBuilder.update_status!(sip_call: sip_call, status: 'accepted', agent: agent)
    update_conversation_call_status(Sip::ConversationCallStatus::IN_PROGRESS)
    create_activity_message(:answered)
    broadcast_accepted
    sip_call
  end

  def reject
    sip_call.reload
    return sip_call if sip_call.terminal? || sip_call.accepted?

    gateway_client.reject_call(sip_call.call_id)
    transition_and_finalize!(
      status: :rejected,
      end_reason: Sip::EndReason::AGENT_REJECTED,
      call_status: Sip::ConversationCallStatus::FAILED,
      activity: :rejected
    )
  end

  def terminate
    return sip_call if sip_call.terminal?

    gateway_client.terminate_call(sip_call.call_id)

    if sip_call.accepted?
      transition_and_finalize!(
        status: :ended,
        end_reason: Sip::EndReason::AGENT_HANGUP,
        call_status: Sip::ConversationCallStatus::COMPLETED,
        activity: :ended
      )
    else
      transition_and_finalize!(
        status: :rejected,
        end_reason: Sip::EndReason::AGENT_CANCELED,
        call_status: Sip::ConversationCallStatus::FAILED,
        activity: :canceled
      )
    end
  end

  private

  def transition_and_finalize!(status:, end_reason:, call_status:, activity:)
    sip_call.update!(status: status, end_reason: end_reason)
    Sip::CallMessageBuilder.update_status!(sip_call: sip_call, status: sip_call.status)
    update_conversation_call_status(call_status)
    create_activity_message(activity)
    broadcast_call_ended
    sip_call
  end

  def ensure_ringing!
    raise Sip::CallErrors::NotRinging, 'Call is not in ringing state' unless sip_call.ringing?
  end

  def ensure_not_already_taken!
    raise Sip::CallErrors::AlreadyAccepted, 'Call already accepted by another agent' if sip_call.accepted?
  end

  def gateway_client
    @gateway_client ||= Sip::GatewayClient.new(sip_call.inbox.channel)
  end

  def update_conversation_call_status(call_status)
    conversation = sip_call.conversation
    attrs = (conversation.additional_attributes || {}).merge('call_status' => call_status)
    conversation.update!(additional_attributes: attrs)
  end

  def broadcast_accepted
    broadcast_event('sip_call.accepted', {
                      account_id: sip_call.account_id,
                      id: sip_call.id,
                      call_id: sip_call.call_id,
                      accepted_by_agent_id: agent.id,
                      conversation_id: sip_call.conversation_id
                    })
  end

  def broadcast_call_ended
    broadcast_event('sip_call.ended', {
                      account_id: sip_call.account_id,
                      id: sip_call.id,
                      call_id: sip_call.call_id,
                      status: sip_call.status,
                      reason: sip_call.end_reason,
                      duration_seconds: sip_call.duration_seconds,
                      conversation_id: sip_call.conversation_id
                    })
  end

  def broadcast_event(event, data)
    ActionCable.server.broadcast("account_#{sip_call.account_id}", { event: event, data: data })
  end

  def create_activity_message(action)
    Sip::ActivityMessageService.new(
      conversation: sip_call.conversation,
      action: action,
      direction: sip_call.direction,
      user: agent,
      call_id: sip_call.call_id
    ).perform
  end
end
