require 'rails_helper'

RSpec.describe Sip::RecordingAttachmentService, type: :service do
  let(:account) { create(:account) }
  let(:inbox) { create(:channel_sip, account: account).inbox }
  let(:contact) { create(:contact, account: account, phone_number: '+15550001111') }
  let(:contact_inbox) { create(:contact_inbox, contact: contact, inbox: inbox, source_id: contact.phone_number.delete('+')) }
  let(:conversation) do
    create(:conversation, account: account, inbox: inbox, contact: contact, contact_inbox: contact_inbox)
  end
  let(:sip_call) do
    create(:sip_call, account: account, inbox: inbox, conversation: conversation, direction: 'inbound')
  end

  before do
    msg = create(
      :message,
      account: account, inbox: inbox, conversation: conversation,
      sender: contact, message_type: 'incoming', content_type: 'voice_call',
      content_attributes: { 'data' => { 'call_sid' => sip_call.call_id, 'status' => 'ringing' } }
    )
    sip_call.update!(message: msg)
  end

  describe '.attach_uploaded_recording!' do
    let(:recording) do
      tempfile = Tempfile.new(['recording', '.wav'])
      tempfile.write('fake-audio-data')
      tempfile.rewind

      ActionDispatch::Http::UploadedFile.new(
        tempfile: tempfile,
        filename: 'call-recording.wav',
        type: 'audio/wav'
      )
    end

    it 'attaches the recording to the sip_call' do
      described_class.attach_uploaded_recording!(sip_call: sip_call, recording: recording)

      expect(sip_call.recording).to be_attached
      expect(sip_call.recording.filename.to_s).to eq('call-recording.wav')
    end

    it 'updates the recording_url in the linked message' do
      described_class.attach_uploaded_recording!(sip_call: sip_call, recording: recording)

      sip_call.message.reload
      expect(sip_call.message.content_attributes.dig('data', 'recording_url')).to be_present
    end

    it 'uses call_id as filename when original_filename is blank' do
      blank_name_recording = ActionDispatch::Http::UploadedFile.new(
        tempfile: recording.tempfile,
        filename: '',
        type: 'audio/wav'
      )

      described_class.attach_uploaded_recording!(sip_call: sip_call, recording: blank_name_recording)

      expect(sip_call.recording.filename.to_s).to eq("#{sip_call.call_id}.wav")
    end

    it 'normalizes application/octet-stream content type to audio/wav' do
      octet_recording = ActionDispatch::Http::UploadedFile.new(
        tempfile: recording.tempfile,
        filename: 'call.wav',
        type: 'application/octet-stream'
      )

      described_class.attach_uploaded_recording!(sip_call: sip_call, recording: octet_recording)

      expect(sip_call.recording.content_type).to eq('audio/wav')
    end

    it 'normalizes blank content type to audio/wav' do
      blank_ct_recording = ActionDispatch::Http::UploadedFile.new(
        tempfile: recording.tempfile,
        filename: 'call.wav',
        type: ''
      )

      described_class.attach_uploaded_recording!(sip_call: sip_call, recording: blank_ct_recording)

      expect(sip_call.recording.content_type).to eq('audio/wav')
    end

    it 'preserves valid content types' do
      mp3_recording = ActionDispatch::Http::UploadedFile.new(
        tempfile: recording.tempfile,
        filename: 'call.mp3',
        type: 'audio/mpeg'
      )

      described_class.attach_uploaded_recording!(sip_call: sip_call, recording: mp3_recording)

      expect(sip_call.recording.content_type).to eq('audio/mpeg')
    end
  end
end
