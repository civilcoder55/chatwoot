require 'rails_helper'

RSpec.describe Sip::IncomingCallService, type: :service do
  let(:account) { create(:account) }
  let(:inbox) { create(:channel_sip, account: account).inbox }
  let(:user) { create(:user, account: account) }
  let(:contact) { create(:contact, account: account, phone_number: '+15550001111') }
  let(:contact_inbox) { create(:contact_inbox, contact: contact, inbox: inbox, source_id: contact.phone_number.delete('+')) }
  let(:conversation) do
    create(:conversation, account: account, inbox: inbox, contact: contact, contact_inbox: contact_inbox)
  end

  before do
    allow(ActionCable.server).to receive(:broadcast)
  end

  def create_sip_call(direction:, status: 'ringing', end_reason: nil, accepted_by_agent: nil)
    sip_call = create(
      :sip_call,
      account: account,
      inbox: inbox,
      conversation: conversation,
      direction: direction,
      status: status,
      end_reason: end_reason,
      accepted_by_agent: accepted_by_agent
    )

    message = create(
      :message,
      account: account,
      inbox: inbox,
      conversation: conversation,
      sender: direction == 'inbound' ? contact : user,
      message_type: direction == 'inbound' ? 'incoming' : 'outgoing',
      content_type: 'voice_call',
      content_attributes: {
        'data' => {
          'call_sid' => sip_call.call_id,
          'status' => Sip::VoiceStatus::SIP_TO_VOICE[status] || status,
          'call_direction' => direction,
          'call_source' => 'sip',
          'sip_call_id' => sip_call.id
        }
      }
    )

    sip_call.update!(message: message)
    sip_call
  end

  def perform_ended(sip_call, reason:, duration_seconds: nil)
    described_class.new(
      inbox: inbox,
      params: {
        event: 'call.ended',
        call_id: sip_call.call_id,
        reason: reason,
        duration_seconds: duration_seconds
      }
    ).perform
  end

  describe 'handle_incoming (call.incoming)' do
    it 'creates a contact, conversation, sip_call and broadcasts the incoming event' do
      params = {
        event: 'call.incoming',
        call_id: 'incoming-call-1',
        from: '+15559998888',
        sdp_offer: 'remote-offer-sdp',
        phone_number: inbox.channel.phone_number
      }

      expect do
        described_class.new(inbox: inbox, params: params).perform
      end.to change(SipCall, :count).by(1)
        .and have_enqueued_job(Sip::CallTimeoutJob)

      sip_call = SipCall.find_by(call_id: 'incoming-call-1')
      expect(sip_call.direction).to eq('inbound')
      expect(sip_call.status).to eq('ringing')
      expect(sip_call.meta['sdp_offer']).to eq('remote-offer-sdp')
      expect(sip_call.message).to be_present
      expect(sip_call.conversation).to be_present

      expect(ActionCable.server).to have_received(:broadcast).with(
        "account_#{account.id}",
        hash_including(event: 'sip_call.incoming')
      )
    end

    it 'reuses an existing unresolved conversation for the same contact' do
      # Force contact_inbox and conversation to exist before the incoming call
      contact_inbox
      conversation

      params = {
        event: 'call.incoming',
        call_id: 'incoming-call-2',
        from: contact.phone_number,
        sdp_offer: 'offer-sdp'
      }

      expect do
        described_class.new(inbox: inbox, params: params).perform
      end.to change(SipCall, :count).by(1)
        .and change(Conversation, :count).by(0)

      sip_call = SipCall.find_by(call_id: 'incoming-call-2')
      expect(sip_call.conversation).to eq(conversation)
    end

    it 'handles duplicate call_id gracefully when DB constraint fires' do
      contact_inbox
      conversation

      # Insert directly to bypass model validation so the DB unique constraint fires
      existing = create(:sip_call, account: account, inbox: inbox, conversation: conversation, call_id: 'dup-call')

      # Stub create! to raise RecordNotUnique as would happen without model validation
      allow(SipCall).to receive(:create!).and_raise(
        ActiveRecord::RecordNotUnique.new("Duplicate call_id: dup-call")
      )

      params = {
        event: 'call.incoming',
        call_id: 'dup-call',
        from: contact.phone_number,
        sdp_offer: 'offer-sdp'
      }

      expect do
        described_class.new(inbox: inbox, params: params).perform
      end.not_to raise_error
    end
  end

  describe 'call.ringing event' do
    it 'broadcasts sip_call.ringing' do
      sip_call = create_sip_call(direction: 'inbound')

      described_class.new(
        inbox: inbox,
        params: { event: 'call.ringing', call_id: sip_call.call_id }
      ).perform

      expect(ActionCable.server).to have_received(:broadcast).with(
        "account_#{account.id}",
        hash_including(event: 'sip_call.ringing')
      )
    end
  end

  describe 'call.accepted / call.answered event' do
    it 'marks the sip_call as accepted and broadcasts sip_call.answered' do
      sip_call = create_sip_call(direction: 'outbound')

      described_class.new(
        inbox: inbox,
        params: { event: 'call.accepted', call_id: sip_call.call_id }
      ).perform

      sip_call.reload
      expect(sip_call.status).to eq('accepted')
      expect(sip_call.conversation.additional_attributes['call_status']).to eq('in-progress')
      expect(ActionCable.server).to have_received(:broadcast).with(
        "account_#{account.id}",
        hash_including(event: 'sip_call.answered')
      )
    end

    it 'handles call.answered the same as call.accepted' do
      sip_call = create_sip_call(direction: 'outbound')

      described_class.new(
        inbox: inbox,
        params: { event: 'call.answered', call_id: sip_call.call_id }
      ).perform

      sip_call.reload
      expect(sip_call.status).to eq('accepted')
    end

    it 'does not downgrade an already accepted call' do
      sip_call = create_sip_call(direction: 'outbound', status: 'accepted')

      described_class.new(
        inbox: inbox,
        params: { event: 'call.accepted', call_id: sip_call.call_id }
      ).perform

      sip_call.reload
      expect(sip_call.status).to eq('accepted')
    end
  end

  describe 'call.sdp_answer event' do
    it 'broadcasts sip_call.sdp_answer with the sdp_answer payload' do
      sip_call = create_sip_call(direction: 'outbound')

      described_class.new(
        inbox: inbox,
        params: { event: 'call.sdp_answer', call_id: sip_call.call_id, sdp_answer: 'remote-answer-sdp' }
      ).perform

      expect(ActionCable.server).to have_received(:broadcast).with(
        "account_#{account.id}",
        hash_including(
          event: 'sip_call.sdp_answer',
          data: hash_including(sdp_answer: 'remote-answer-sdp')
        )
      )
    end
  end

  describe 'unknown event' do
    it 'logs a warning and does nothing' do
      allow(Rails.logger).to receive(:warn)

      described_class.new(
        inbox: inbox,
        params: { event: 'call.unknown_event', call_id: 'whatever' }
      ).perform

      expect(Rails.logger).to have_received(:warn).with(/Unknown event: call.unknown_event/)
    end
  end

  describe 'call.ended event' do
    it 'maps busy to rejected and logs a busy activity message' do
      sip_call = create_sip_call(direction: 'outbound')

      expect do
        perform_ended(sip_call, reason: 'busy')
      end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
        conversation,
        hash_including(content: "Recipient was busy (#{sip_call.call_id})")
      )

      sip_call.reload
      conversation.reload

      expect(sip_call.status).to eq('rejected')
      expect(sip_call.end_reason).to eq('busy')
      expect(conversation.additional_attributes['call_status']).to eq('failed')
    end

    it 'maps rejected to rejected and logs a remote rejection activity message' do
      sip_call = create_sip_call(direction: 'outbound')

      expect do
        perform_ended(sip_call, reason: 'rejected')
      end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
        conversation,
        hash_including(content: "Recipient rejected the call (#{sip_call.call_id})")
      )

      sip_call.reload
      expect(sip_call.status).to eq('rejected')
      expect(sip_call.end_reason).to eq('rejected')
    end

    it 'normalizes caller-hangup to remote-canceled and logs a cancellation activity message' do
      sip_call = create_sip_call(direction: 'outbound')

      expect do
        perform_ended(sip_call, reason: 'caller-hangup')
      end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
        conversation,
        hash_including(content: "Recipient canceled the call (#{sip_call.call_id})")
      )

      sip_call.reload
      conversation.reload

      expect(sip_call.status).to eq('missed')
      expect(sip_call.end_reason).to eq('remote-canceled')
      expect(conversation.additional_attributes['call_status']).to eq('no-answer')
    end

    it 'maps no-answer to missed and logs the no-answer activity message' do
      sip_call = create_sip_call(direction: 'outbound')

      expect do
        perform_ended(sip_call, reason: 'no-answer')
      end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
        conversation,
        hash_including(content: "The call was not answered (#{sip_call.call_id})")
      )

      sip_call.reload
      expect(sip_call.status).to eq('missed')
      expect(sip_call.end_reason).to eq('no-answer')
    end

    it 'maps remote hangups on connected calls to ended and logs the remote-ended activity message' do
      sip_call = create_sip_call(direction: 'outbound', status: 'accepted')

      expect do
        perform_ended(sip_call, reason: 'remote-hangup', duration_seconds: 42)
      end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
        conversation,
        hash_including(content: "Recipient ended the call (#{sip_call.call_id})")
      )

      sip_call.reload
      conversation.reload

      expect(sip_call.status).to eq('ended')
      expect(sip_call.end_reason).to eq('remote-hangup')
      expect(sip_call.duration_seconds).to eq(42)
      expect(conversation.additional_attributes['call_status']).to eq('completed')
    end

    it 'maps SIP failure codes to failed and logs a generic failure activity message' do
      sip_call = create_sip_call(direction: 'outbound')

      expect do
        perform_ended(sip_call, reason: 'sip-500')
      end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
        conversation,
        hash_including(content: "Call failed (#{sip_call.call_id})")
      )

      sip_call.reload
      expect(sip_call.status).to eq('failed')
      expect(sip_call.end_reason).to eq('sip-500')
    end

    it 'does not overwrite an agent-owned terminal reason and only syncs webhook metadata' do
      sip_call = create_sip_call(direction: 'outbound', status: 'rejected', end_reason: 'agent-canceled')

      expect do
        perform_ended(sip_call, reason: 'agent-canceled', duration_seconds: 12)
      end.not_to have_enqueued_job(Conversations::ActivityMessageJob)

      sip_call.reload
      sip_call.message.reload

      expect(sip_call.status).to eq('rejected')
      expect(sip_call.end_reason).to eq('agent-canceled')
      expect(sip_call.duration_seconds).to eq(12)
      expect(sip_call.message.content_attributes.dig('data', 'duration_seconds')).to eq(12)
    end

    it 'normalizes gateway-hangup on a ringing call to agent-canceled' do
      sip_call = create_sip_call(direction: 'outbound')

      perform_ended(sip_call, reason: 'gateway-hangup')

      sip_call.reload
      expect(sip_call.status).to eq('rejected')
      expect(sip_call.end_reason).to eq('agent-canceled')
    end

    it 'normalizes gateway-hangup on a connected call to agent-hangup' do
      sip_call = create_sip_call(direction: 'outbound', status: 'accepted')

      perform_ended(sip_call, reason: 'gateway-hangup', duration_seconds: 30)

      sip_call.reload
      expect(sip_call.status).to eq('ended')
      expect(sip_call.end_reason).to eq('agent-hangup')
      expect(sip_call.duration_seconds).to eq(30)
    end

    it 'maps service-unavailable to failed' do
      sip_call = create_sip_call(direction: 'outbound')

      perform_ended(sip_call, reason: 'service-unavailable')

      sip_call.reload
      expect(sip_call.status).to eq('failed')
      expect(sip_call.end_reason).to eq('service-unavailable')
    end

    it 'maps an unknown reason on a ringing call to failed' do
      sip_call = create_sip_call(direction: 'outbound')

      perform_ended(sip_call, reason: 'something-unexpected')

      sip_call.reload
      expect(sip_call.status).to eq('failed')
      expect(sip_call.end_reason).to eq('failed')
    end

    it 'maps an unknown reason on a connected call to remote-hangup (ended)' do
      sip_call = create_sip_call(direction: 'outbound', status: 'accepted')

      perform_ended(sip_call, reason: 'something-unexpected', duration_seconds: 10)

      sip_call.reload
      expect(sip_call.status).to eq('ended')
      expect(sip_call.end_reason).to eq('remote-hangup')
    end

    it 'does nothing when the sip_call is not found' do
      expect do
        described_class.new(
          inbox: inbox,
          params: { event: 'call.ended', call_id: 'non-existent', reason: 'no-answer' }
        ).perform
      end.not_to raise_error
    end

    it 'broadcasts sip_call.ended after finalizing the call' do
      sip_call = create_sip_call(direction: 'outbound')

      perform_ended(sip_call, reason: 'no-answer')

      expect(ActionCable.server).to have_received(:broadcast).with(
        "account_#{account.id}",
        hash_including(event: 'sip_call.ended')
      )
    end
  end
end
