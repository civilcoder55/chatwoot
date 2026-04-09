require 'rails_helper'

RSpec.describe Sip::CallService, type: :service do
  let(:account) { create(:account) }
  let(:inbox) { create(:channel_sip, account: account).inbox }
  let(:user) { create(:user, account: account) }
  let(:contact) { create(:contact, account: account, phone_number: '+15550001111') }
  let(:contact_inbox) { create(:contact_inbox, contact: contact, inbox: inbox, source_id: contact.phone_number.delete('+')) }
  let(:conversation) do
    create(:conversation, account: account, inbox: inbox, contact: contact, contact_inbox: contact_inbox)
  end
  let(:gateway_client) { instance_double(Sip::GatewayClient, accept_call: true, reject_call: true, terminate_call: true) }

  before do
    allow(Sip::GatewayClient).to receive(:new).and_return(gateway_client)
    allow(ActionCable.server).to receive(:broadcast)
  end

  def create_voice_call_message(sip_call, message_type:)
    create(
      :message,
      account: account,
      inbox: inbox,
      conversation: conversation,
      sender: message_type == 'incoming' ? contact : user,
      message_type: message_type,
      content_type: 'voice_call',
      content_attributes: {
        'data' => {
          'call_sid' => sip_call.call_id,
          'status' => 'ringing',
          'call_direction' => sip_call.direction,
          'call_source' => 'sip',
          'sip_call_id' => sip_call.id
        }
      }
    )
  end

  describe '.initiate' do
    let(:gateway_result) { { 'call_id' => 'outbound-call-1', 'sdp_answer' => 'gateway-answer-sdp' } }

    before do
      allow(gateway_client).to receive(:initiate_call).and_return(gateway_result)
    end

    it 'creates an outbound sip_call with ringing status and links a voice_call message' do
      sip_call = described_class.initiate(conversation: conversation, agent: user, sdp_offer: 'offer-sdp')

      expect(sip_call).to be_persisted
      expect(sip_call.direction).to eq('outbound')
      expect(sip_call.status).to eq('ringing')
      expect(sip_call.call_id).to eq('outbound-call-1')
      expect(sip_call.meta['sdp_offer']).to eq('offer-sdp')
      expect(sip_call.meta['sdp_answer']).to eq('gateway-answer-sdp')
      expect(sip_call.message).to be_present
      expect(sip_call.message.content_type).to eq('voice_call')
    end

    it 'raises CallFailed when inbox is not a SIP channel' do
      non_sip_inbox = create(:inbox, account: account)
      non_sip_conversation = create(:conversation, account: account, inbox: non_sip_inbox, contact: contact)

      expect do
        described_class.initiate(conversation: non_sip_conversation, agent: user, sdp_offer: 'offer-sdp')
      end.to raise_error(Sip::CallErrors::CallFailed, 'Not a SIP inbox')
    end

    it 'raises CallFailed when contact has no phone number' do
      no_phone_contact = create(:contact, account: account, phone_number: nil)
      no_phone_ci = create(:contact_inbox, contact: no_phone_contact, inbox: inbox)
      no_phone_conversation = create(:conversation, account: account, inbox: inbox, contact: no_phone_contact, contact_inbox: no_phone_ci)

      expect do
        described_class.initiate(conversation: no_phone_conversation, agent: user, sdp_offer: 'offer-sdp')
      end.to raise_error(Sip::CallErrors::CallFailed, 'Contact phone number not available')
    end

    it 'calls the gateway with correct from/to/sdp_offer' do
      described_class.initiate(conversation: conversation, agent: user, sdp_offer: 'offer-sdp')

      expect(gateway_client).to have_received(:initiate_call).with(
        from: inbox.channel.phone_number,
        to: contact.phone_number,
        sdp_offer: 'offer-sdp'
      )
    end
  end

  describe '#accept' do
    it 'marks the call accepted and enqueues an answered activity message' do
      sip_call = create(:sip_call, account: account, inbox: inbox, conversation: conversation, direction: 'inbound')
      message = create_voice_call_message(sip_call, message_type: 'incoming')
      sip_call.update!(message: message)

      expect do
        described_class.new(sip_call: sip_call, agent: user).accept('answer-sdp')
      end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
        conversation,
        hash_including(content: "#{user.name} answered the call (#{sip_call.call_id})")
      )

      sip_call.reload
      message.reload
      conversation.reload

      expect(sip_call.status).to eq('accepted')
      expect(sip_call.accepted_by_agent_id).to eq(user.id)
      expect(conversation.additional_attributes['call_status']).to eq('in-progress')
      expect(message.content_attributes.dig('data', 'status')).to eq('in-progress')
      expect(message.content_attributes.dig('data', 'accepted_by')).to eq({ 'id' => user.id, 'name' => user.name })
      expect(ActionCable.server).to have_received(:broadcast).with(
        "account_#{account.id}",
        hash_including(event: 'sip_call.accepted')
      )
    end

    it 'raises NotRinging when the call is not in ringing state' do
      sip_call = create(:sip_call, account: account, inbox: inbox, conversation: conversation,
                        direction: 'inbound', status: 'accepted', accepted_by_agent: user)
      message = create_voice_call_message(sip_call, message_type: 'incoming')
      sip_call.update!(message: message)

      expect do
        described_class.new(sip_call: sip_call, agent: user).accept('answer-sdp')
      end.to raise_error(Sip::CallErrors::NotRinging, 'Call is not in ringing state')
    end

    it 'calls gateway accept_call with correct arguments' do
      sip_call = create(:sip_call, account: account, inbox: inbox, conversation: conversation, direction: 'inbound')
      message = create_voice_call_message(sip_call, message_type: 'incoming')
      sip_call.update!(message: message)

      described_class.new(sip_call: sip_call, agent: user).accept('answer-sdp')

      expect(gateway_client).to have_received(:accept_call).with(sip_call.call_id, 'answer-sdp')
    end
  end

  describe '#reject' do
    it 'stores the canonical rejected reason and enqueues a reject activity message' do
      sip_call = create(:sip_call, account: account, inbox: inbox, conversation: conversation, direction: 'inbound')
      message = create_voice_call_message(sip_call, message_type: 'incoming')
      sip_call.update!(message: message)

      expect do
        described_class.new(sip_call: sip_call, agent: user).reject
      end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
        conversation,
        hash_including(content: "#{user.name} rejected the call (#{sip_call.call_id})")
      )

      sip_call.reload
      message.reload
      conversation.reload

      expect(sip_call.status).to eq('rejected')
      expect(sip_call.end_reason).to eq('agent-rejected')
      expect(conversation.additional_attributes['call_status']).to eq('failed')
      expect(message.content_attributes.dig('data', 'status')).to eq('failed')
      expect(ActionCable.server).to have_received(:broadcast).with(
        "account_#{account.id}",
        hash_including(
          event: 'sip_call.ended',
          data: hash_including(reason: 'agent-rejected', status: 'rejected')
        )
      )
    end

    it 'returns the sip_call unchanged when already in terminal state' do
      sip_call = create(:sip_call, account: account, inbox: inbox, conversation: conversation,
                        direction: 'inbound', status: 'ended', end_reason: 'agent-hangup')
      message = create_voice_call_message(sip_call, message_type: 'incoming')
      sip_call.update!(message: message)

      result = described_class.new(sip_call: sip_call, agent: user).reject

      expect(result.status).to eq('ended')
      expect(gateway_client).not_to have_received(:reject_call)
    end

    it 'returns the sip_call unchanged when already accepted' do
      sip_call = create(:sip_call, account: account, inbox: inbox, conversation: conversation,
                        direction: 'inbound', status: 'accepted', accepted_by_agent: user)
      message = create_voice_call_message(sip_call, message_type: 'incoming')
      sip_call.update!(message: message)

      result = described_class.new(sip_call: sip_call, agent: user).reject

      expect(result.status).to eq('accepted')
      expect(gateway_client).not_to have_received(:reject_call)
    end
  end

  describe '#terminate' do
    it 'treats a ringing termination as an agent cancellation' do
      sip_call = create(:sip_call, account: account, inbox: inbox, conversation: conversation, direction: 'outbound')
      message = create_voice_call_message(sip_call, message_type: 'outgoing')
      sip_call.update!(message: message)

      expect do
        described_class.new(sip_call: sip_call, agent: user).terminate
      end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
        conversation,
        hash_including(content: "#{user.name} canceled the call (#{sip_call.call_id})")
      )

      sip_call.reload
      message.reload
      conversation.reload

      expect(sip_call.status).to eq('rejected')
      expect(sip_call.end_reason).to eq('agent-canceled')
      expect(conversation.additional_attributes['call_status']).to eq('failed')
      expect(message.content_attributes.dig('data', 'status')).to eq('failed')
      expect(ActionCable.server).to have_received(:broadcast).with(
        "account_#{account.id}",
        hash_including(
          event: 'sip_call.ended',
          data: hash_including(reason: 'agent-canceled', status: 'rejected')
        )
      )
    end

    it 'treats a connected termination as an agent hangup' do
      sip_call = create(
        :sip_call,
        account: account,
        inbox: inbox,
        conversation: conversation,
        direction: 'inbound',
        status: 'accepted',
        accepted_by_agent: user
      )
      message = create_voice_call_message(sip_call, message_type: 'incoming')
      sip_call.update!(message: message)

      expect do
        described_class.new(sip_call: sip_call, agent: user).terminate
      end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
        conversation,
        hash_including(content: "#{user.name} ended the call (#{sip_call.call_id})")
      )

      sip_call.reload
      message.reload
      conversation.reload

      expect(sip_call.status).to eq('ended')
      expect(sip_call.end_reason).to eq('agent-hangup')
      expect(conversation.additional_attributes['call_status']).to eq('completed')
      expect(message.content_attributes.dig('data', 'status')).to eq('completed')
      expect(ActionCable.server).to have_received(:broadcast).with(
        "account_#{account.id}",
        hash_including(
          event: 'sip_call.ended',
          data: hash_including(reason: 'agent-hangup', status: 'ended')
        )
      )
    end

    it 'returns the sip_call unchanged when already in terminal state' do
      sip_call = create(:sip_call, account: account, inbox: inbox, conversation: conversation,
                        direction: 'inbound', status: 'ended', end_reason: 'agent-hangup')

      result = described_class.new(sip_call: sip_call, agent: user).terminate

      expect(result.status).to eq('ended')
      expect(gateway_client).not_to have_received(:terminate_call)
    end
  end
end
