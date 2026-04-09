class Sip::IncomingCallService
  pattr_initialize [:inbox!, :params!]

  CALL_TIMEOUT_SECONDS = 30

  WEBHOOK_INCOMING   = 'call.incoming'.freeze
  WEBHOOK_RINGING    = 'call.ringing'.freeze
  WEBHOOK_ACCEPTED   = 'call.accepted'.freeze
  WEBHOOK_ANSWERED   = 'call.answered'.freeze
  WEBHOOK_SDP_ANSWER = 'call.sdp_answer'.freeze
  WEBHOOK_ENDED      = 'call.ended'.freeze

  EVENT_INCOMING   = 'sip_call.incoming'.freeze
  EVENT_RINGING    = 'sip_call.ringing'.freeze
  EVENT_ANSWERED   = 'sip_call.answered'.freeze
  EVENT_SDP_ANSWER = 'sip_call.sdp_answer'.freeze
  EVENT_ENDED      = 'sip_call.ended'.freeze

  def perform
    case params[:event]
    when WEBHOOK_INCOMING then handle_incoming
    when WEBHOOK_RINGING then broadcast_sip_call_event(EVENT_RINGING)
    when WEBHOOK_ACCEPTED, WEBHOOK_ANSWERED then mark_call_connected
    when WEBHOOK_SDP_ANSWER then broadcast_sip_call_event(EVENT_SDP_ANSWER, sdp_answer: params[:sdp_answer])
    when WEBHOOK_ENDED then handle_ended
    else Rails.logger.warn "[SIP CALL] Unknown event: #{params[:event]}"
    end
  end

  private

  # --- Incoming call flow ---

  def handle_incoming
    contact = find_or_create_contact(params[:from])
    return unless contact

    conversation = find_or_create_conversation(contact)
    return unless conversation

    sip_call = persist_incoming_call(conversation)
    broadcast_incoming_call(sip_call, contact)
    Sip::CallTimeoutJob.set(wait: CALL_TIMEOUT_SECONDS.seconds).perform_later(sip_call.id)
  rescue ActiveRecord::RecordNotUnique
    Rails.logger.warn "[SIP CALL] Duplicate call_id: #{params[:call_id]}"
  end

  def persist_incoming_call(conversation)
    ActiveRecord::Base.transaction do
      sip_call = create_call_record(conversation)
      link_voice_call_message(conversation, sip_call)
      update_conversation_call_status(conversation, Sip::ConversationCallStatus::RINGING, sip_call.direction)
      sip_call
    end
  end

  # --- Call ended flow ---

  def handle_ended
    sip_call = find_sip_call
    return unless sip_call

    duration = params[:duration_seconds]&.to_i
    canonical_reason = normalize_end_reason(sip_call, params[:reason].to_s, duration)

    if sip_call.terminal? && sip_call.end_reason.present?
      sync_terminal_metadata(sip_call, duration)
      return
    end

    finalize_call(sip_call, canonical_reason, duration)
  end

  def finalize_call(sip_call, canonical_reason, duration)
    final_status = SipCall.status_for_end_reason(canonical_reason)

    ActiveRecord::Base.transaction do
      sip_call.update!(status: final_status, duration_seconds: duration, end_reason: canonical_reason)
      Sip::CallMessageBuilder.update_status!(
        sip_call: sip_call, status: final_status,
        agent: accepted_agent(sip_call), duration_seconds: duration
      )
      update_conversation_call_status(sip_call.conversation, map_voice_status(final_status), sip_call.direction)
      create_terminal_activity_message(sip_call, canonical_reason)
    end

    broadcast_call_ended(sip_call)
  end

  # --- Call connected flow ---

  def mark_call_connected
    sip_call = find_sip_call
    return unless sip_call

    ActiveRecord::Base.transaction do
      sip_call.update!(status: :accepted) unless sip_call.accepted?
      Sip::CallMessageBuilder.update_status!(sip_call: sip_call, status: 'accepted')
      update_conversation_call_status(sip_call.conversation, Sip::ConversationCallStatus::IN_PROGRESS, sip_call.direction)
    end

    broadcast_sip_call_event(EVENT_ANSWERED, sip_call: sip_call)
  end

  # --- Persistence helpers ---

  def find_sip_call
    SipCall.find_by(call_id: params[:call_id])
  end

  def create_call_record(conversation)
    SipCall.create!(
      account: inbox.account,
      inbox: inbox,
      conversation: conversation,
      call_id: params[:call_id],
      direction: :inbound,
      status: :ringing,
      meta: { 'sdp_offer' => params[:sdp_offer], 'ice_servers' => default_ice_servers }
    )
  end

  def link_voice_call_message(conversation, sip_call)
    message = Sip::CallMessageBuilder.create!(conversation: conversation, sip_call: sip_call)
    sip_call.update!(message_id: message.id)
  end

  def sync_terminal_metadata(sip_call, duration)
    return unless duration.present? && sip_call.duration_seconds != duration

    ActiveRecord::Base.transaction do
      sip_call.update!(duration_seconds: duration)
      Sip::CallMessageBuilder.update_status!(
        sip_call: sip_call, status: sip_call.status,
        agent: accepted_agent(sip_call), duration_seconds: sip_call.duration_seconds
      )
    end
  end

  def update_conversation_call_status(conversation, call_status, direction)
    attrs = (conversation.additional_attributes || {}).merge(
      'call_status' => call_status,
      'call_direction' => direction
    )
    conversation.update!(additional_attributes: attrs)
  end

  def create_terminal_activity_message(sip_call, reason)
    Sip::ActivityMessageService.new(
      conversation: sip_call.conversation,
      action: :terminal,
      direction: sip_call.direction,
      reason: reason,
      call_id: sip_call.call_id
    ).perform
  end

  # --- Contact / Conversation lookup ---

  def find_or_create_contact(phone_number)
    normalized = phone_number.start_with?('+') ? phone_number : "+#{phone_number}"
    source_id = normalized.delete('+')

    contact_inbox = ::ContactInboxWithContactBuilder.new(
      source_id: source_id,
      inbox: inbox,
      contact_attributes: { name: normalized, phone_number: normalized }
    ).perform

    contact_inbox&.contact
  end

  def find_or_create_conversation(contact)
    contact_inbox = contact.contact_inboxes.find_by(inbox: inbox)
    return unless contact_inbox

    contact_inbox.conversations.where.not(status: :resolved).last ||
      ::Conversation.create!(account_id: inbox.account_id, inbox: inbox, contact: contact, contact_inbox: contact_inbox)
  end

  # --- End reason resolution ---

  def normalize_end_reason(sip_call, reason, duration)
    case reason
    when Sip::EndReason::CALLER_HANGUP
      Sip::EndReason::REMOTE_CANCELED
    when Sip::EndReason::GATEWAY_HANGUP
      call_connected?(sip_call, duration) ? Sip::EndReason::AGENT_HANGUP : Sip::EndReason::AGENT_CANCELED
    else
      return reason if SipCall.known_end_reason?(reason)

      call_connected?(sip_call, duration) ? Sip::EndReason::REMOTE_HANGUP : Sip::EndReason::FAILED_STATUS
    end
  end

  def call_connected?(sip_call, duration)
    sip_call.accepted? || duration.to_i.positive? || sip_call.accepted_by_agent_id.present?
  end

  # --- Utility ---

  def accepted_agent(sip_call)
    sip_call.accepted_by_agent if sip_call.accepted_by_agent_id.present?
  end

  def map_voice_status(status)
    Sip::VoiceStatus::SIP_TO_VOICE[status] || status
  end

  # --- Broadcasting ---

  def broadcast_incoming_call(sip_call, contact)
    broadcast_event(EVENT_INCOMING,
                    account_id: inbox.account_id,
                    id: sip_call.id,
                    call_id: sip_call.call_id,
                    direction: sip_call.direction,
                    inbox_id: sip_call.inbox_id,
                    conversation_id: sip_call.conversation_id,
                    caller: { name: contact.name, phone: contact.phone_number, avatar: contact.avatar_url },
                    sdp_offer: params[:sdp_offer],
                    ice_servers: default_ice_servers)
  end

  def broadcast_call_ended(sip_call)
    broadcast_event(EVENT_ENDED,
                    account_id: sip_call.account_id,
                    id: sip_call.id,
                    call_id: sip_call.call_id,
                    status: sip_call.status,
                    reason: sip_call.end_reason,
                    duration_seconds: sip_call.duration_seconds,
                    conversation_id: sip_call.conversation_id)
  end

  def broadcast_sip_call_event(event, sip_call: nil, **extra)
    sip_call ||= find_sip_call
    return unless sip_call

    broadcast_event(event,
                    account_id: sip_call.account_id, id: sip_call.id,
                    call_id: sip_call.call_id, conversation_id: sip_call.conversation_id,
                    **extra)
  end

  def broadcast_event(event, **data)
    ActionCable.server.broadcast("account_#{inbox.account_id}", { event: event, data: data })
  end

  def default_ice_servers
    [{ urls: 'stun:stun.l.google.com:19302' }]
  end
end
