require 'rails_helper'

RSpec.describe Sip::CallMessageBuilder, type: :service do
  let(:account) { create(:account) }
  let(:inbox) { create(:channel_sip, account: account).inbox }
  let(:user) { create(:user, account: account) }
  let(:contact) { create(:contact, account: account, phone_number: '+15550001111') }
  let(:contact_inbox) { create(:contact_inbox, contact: contact, inbox: inbox, source_id: contact.phone_number.delete('+')) }
  let(:conversation) do
    create(:conversation, account: account, inbox: inbox, contact: contact, contact_inbox: contact_inbox)
  end

  describe '.create!' do
    context 'with an inbound call' do
      let(:sip_call) do
        create(:sip_call, account: account, inbox: inbox, conversation: conversation, direction: 'inbound', status: 'ringing')
      end

      it 'creates a voice_call message with incoming message_type' do
        message = described_class.create!(conversation: conversation, sip_call: sip_call)

        expect(message).to be_persisted
        expect(message.content_type).to eq('voice_call')
        expect(message.message_type).to eq('incoming')
        expect(message.sender).to eq(contact)
      end

      it 'populates the data payload with call details' do
        message = described_class.create!(conversation: conversation, sip_call: sip_call)
        data = message.content_attributes['data']

        expect(data['call_sid']).to eq(sip_call.call_id)
        expect(data['status']).to eq('ringing')
        expect(data['call_direction']).to eq('inbound')
        expect(data['call_source']).to eq('sip')
        expect(data['sip_call_id']).to eq(sip_call.id)
        expect(data['from_number']).to eq(contact.phone_number)
        expect(data['to_number']).to eq(inbox.channel.phone_number)
        expect(data['meta']['created_at']).to be_present
      end
    end

    context 'with an outbound call' do
      let(:sip_call) do
        create(:sip_call, account: account, inbox: inbox, conversation: conversation, direction: 'outbound', status: 'ringing')
      end

      it 'creates a voice_call message with outgoing message_type and agent sender' do
        message = described_class.create!(conversation: conversation, sip_call: sip_call, user: user)

        expect(message.message_type).to eq('outgoing')
        expect(message.sender).to eq(user)
      end

      it 'sets from_number to inbox channel phone and to_number to contact phone' do
        message = described_class.create!(conversation: conversation, sip_call: sip_call, user: user)
        data = message.content_attributes['data']

        expect(data['from_number']).to eq(inbox.channel.phone_number)
        expect(data['to_number']).to eq(contact.phone_number)
      end

      it 'falls back to contact sender when no user provided for outbound' do
        message = described_class.create!(conversation: conversation, sip_call: sip_call)

        expect(message.sender).to eq(contact)
      end
    end
  end

  describe '.update_status!' do
    let(:sip_call) do
      create(:sip_call, account: account, inbox: inbox, conversation: conversation, direction: 'inbound')
    end
    let!(:voice_message) do
      msg = create(
        :message,
        account: account, inbox: inbox, conversation: conversation,
        sender: contact, message_type: 'incoming', content_type: 'voice_call',
        content_attributes: { 'data' => { 'call_sid' => sip_call.call_id, 'status' => 'ringing' } }
      )
      sip_call.update!(message: msg)
      msg
    end

    it 'updates the status in the message content_attributes' do
      described_class.update_status!(sip_call: sip_call, status: 'accepted')

      voice_message.reload
      expect(voice_message.content_attributes.dig('data', 'status')).to eq('in-progress')
    end

    it 'sets accepted_by when agent is provided' do
      described_class.update_status!(sip_call: sip_call, status: 'accepted', agent: user)

      voice_message.reload
      expect(voice_message.content_attributes.dig('data', 'accepted_by')).to eq({ 'id' => user.id, 'name' => user.name })
    end

    it 'sets duration_seconds when provided' do
      described_class.update_status!(sip_call: sip_call, status: 'ended', duration_seconds: 120)

      voice_message.reload
      expect(voice_message.content_attributes.dig('data', 'duration_seconds')).to eq(120)
    end

    it 'returns nil when sip_call has no linked message' do
      sip_call.update!(message: nil)

      result = described_class.update_status!(sip_call: sip_call, status: 'accepted')
      expect(result).to be_nil
    end

    it 'maps SIP statuses to voice statuses' do
      described_class.update_status!(sip_call: sip_call, status: 'rejected')

      voice_message.reload
      expect(voice_message.content_attributes.dig('data', 'status')).to eq('failed')
    end
  end

  describe '.update_recording_url!' do
    let(:sip_call) do
      create(:sip_call, account: account, inbox: inbox, conversation: conversation, direction: 'inbound')
    end
    let!(:voice_message) do
      msg = create(
        :message,
        account: account, inbox: inbox, conversation: conversation,
        sender: contact, message_type: 'incoming', content_type: 'voice_call',
        content_attributes: { 'data' => { 'call_sid' => sip_call.call_id, 'status' => 'ringing' } }
      )
      sip_call.update!(message: msg)
      msg
    end

    it 'sets recording_url in the message content_attributes when recording is attached' do
      sip_call.recording.attach(
        io: StringIO.new('fake-audio-data'),
        filename: 'test.wav',
        content_type: 'audio/wav'
      )

      described_class.update_recording_url!(sip_call: sip_call)

      voice_message.reload
      expect(voice_message.content_attributes.dig('data', 'recording_url')).to be_present
    end

    it 'does nothing when sip_call has no linked message' do
      sip_call.update!(message: nil)

      expect { described_class.update_recording_url!(sip_call: sip_call) }.not_to raise_error
    end
  end
end
