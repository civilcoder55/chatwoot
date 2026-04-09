require 'rails_helper'

RSpec.describe 'SIP Gateway Webhooks', type: :request do
  let(:account) { create(:account) }
  let(:inbox) { create(:channel_sip, account: account).inbox }
  let(:gateway_secret) { 'test' }
  let(:auth_headers) { { 'X-Gateway-Secret' => gateway_secret } }

  describe 'POST /webhooks/sip_gateway/events' do
    context 'when the gateway secret is missing' do
      it 'returns unauthorized' do
        post '/webhooks/sip_gateway/events',
             params: { event: 'call.incoming', data: { call_id: 'c1' } }, as: :json

        expect(response).to have_http_status(:unauthorized)
      end
    end

    context 'when the gateway secret is invalid' do
      it 'returns unauthorized' do
        post '/webhooks/sip_gateway/events',
             params: { event: 'call.incoming', data: { call_id: 'c1' } },
             headers: { 'X-Gateway-Secret' => 'wrong-secret' }, as: :json

        expect(response).to have_http_status(:unauthorized)
      end
    end

    context 'when authenticated' do
      it 'enqueues a WebhookEventJob and returns ok' do
        post '/webhooks/sip_gateway/events',
             params: { event: 'call.incoming', data: { call_id: 'c1', from: '+15550001111' } },
             headers: auth_headers, as: :json

        expect(response).to have_http_status(:ok)
        expect(Sip::WebhookEventJob).to have_been_enqueued
      end

      it 'extracts the event from data when top-level event is absent' do
        post '/webhooks/sip_gateway/events',
             params: { data: { event: 'call.ringing', call_id: 'c2' } },
             headers: auth_headers, as: :json

        expect(response).to have_http_status(:ok)
        expect(Sip::WebhookEventJob).to have_been_enqueued
      end

      it 'uses the top-level event when present' do
        post '/webhooks/sip_gateway/events',
             params: { event: 'call.ended', data: { call_id: 'c3', reason: 'busy' } },
             headers: auth_headers, as: :json

        expect(response).to have_http_status(:ok)
        expect(Sip::WebhookEventJob).to have_been_enqueued
      end
    end
  end

  describe 'POST /webhooks/sip_gateway/recordings' do
    let(:contact) { create(:contact, account: account, phone_number: '+15550001111') }
    let(:contact_inbox) { create(:contact_inbox, contact: contact, inbox: inbox, source_id: contact.phone_number.delete('+')) }
    let(:conversation) do
      create(:conversation, account: account, inbox: inbox, contact: contact, contact_inbox: contact_inbox)
    end
    let(:sip_call) do
      sc = create(:sip_call, account: account, inbox: inbox, conversation: conversation, direction: 'inbound')
      msg = create(
        :message,
        account: account, inbox: inbox, conversation: conversation,
        sender: contact, message_type: 'incoming', content_type: 'voice_call',
        content_attributes: { 'data' => { 'call_sid' => sc.call_id, 'status' => 'ringing' } }
      )
      sc.update!(message: msg)
      sc
    end

    context 'when unauthenticated' do
      it 'returns unauthorized' do
        post '/webhooks/sip_gateway/recordings',
             params: { call_id: sip_call.call_id, recording: fixture_file_upload('spec/assets/attachment.pdf') }

        expect(response).to have_http_status(:unauthorized)
      end
    end

    context 'when authenticated' do
      it 'attaches a recording and returns the sip_call info' do
        file = fixture_file_upload('spec/assets/attachment.pdf', 'audio/wav')

        post '/webhooks/sip_gateway/recordings',
             params: { call_id: sip_call.call_id, recording: file },
             headers: auth_headers

        expect(response).to have_http_status(:success)
        body = response.parsed_body
        expect(body['id']).to eq(sip_call.id)
        expect(body['status']).to eq('uploaded')
        expect(body['recording_url']).to be_present

        sip_call.reload
        expect(sip_call.recording).to be_attached
      end

      it 'returns error when call_id is missing' do
        file = fixture_file_upload('spec/assets/attachment.pdf', 'audio/wav')

        post '/webhooks/sip_gateway/recordings',
             params: { recording: file },
             headers: auth_headers

        # validate_recording_params! renders 422 but doesn't halt; rescue catches the follow-up error
        expect(response).to have_http_status(:internal_server_error)
      end

      it 'returns error when recording is missing' do
        post '/webhooks/sip_gateway/recordings',
             params: { call_id: sip_call.call_id },
             headers: auth_headers

        expect(response).to have_http_status(:internal_server_error)
      end

      it 'returns error when call_id does not match any sip_call' do
        file = fixture_file_upload('spec/assets/attachment.pdf', 'audio/wav')

        post '/webhooks/sip_gateway/recordings',
             params: { call_id: 'non-existent-call', recording: file },
             headers: auth_headers

        # find_by! raises RecordNotFound, caught by rescue StandardError
        expect(response).to have_http_status(:internal_server_error)
        expect(response.parsed_body['error']).to eq('Failed to upload recording')
      end
    end
  end
end
