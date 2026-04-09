require 'rails_helper'

RSpec.describe 'SIP Calls API', type: :request do
  let(:account) { create(:account) }
  let(:admin) { create(:user, account: account, role: :administrator) }
  let(:agent) { create(:user, account: account, role: :agent) }
  let(:inbox) { create(:channel_sip, account: account).inbox }
  let(:contact) { create(:contact, account: account, phone_number: '+15550001111') }
  let(:contact_inbox) { create(:contact_inbox, contact: contact, inbox: inbox, source_id: contact.phone_number.delete('+')) }
  let(:conversation) do
    create(:conversation, account: account, inbox: inbox, contact: contact, contact_inbox: contact_inbox)
  end

  let(:gateway_client) do
    instance_double(Sip::GatewayClient, accept_call: true, reject_call: true, terminate_call: true, initiate_call: nil)
  end

  before do
    create(:inbox_member, user: agent, inbox: inbox)
    allow(Sip::GatewayClient).to receive(:new).and_return(gateway_client)
    allow(ActionCable.server).to receive(:broadcast)
  end

  def create_sip_call_with_message(direction: 'inbound', status: 'ringing')
    sip_call = create(:sip_call, account: account, inbox: inbox, conversation: conversation,
                                 direction: direction, status: status)
    message = create(
      :message,
      account: account, inbox: inbox, conversation: conversation,
      sender: direction == 'inbound' ? contact : agent,
      message_type: direction == 'inbound' ? 'incoming' : 'outgoing',
      content_type: 'voice_call',
      content_attributes: {
        'data' => {
          'call_sid' => sip_call.call_id, 'status' => 'ringing',
          'call_direction' => direction, 'call_source' => 'sip', 'sip_call_id' => sip_call.id
        }
      }
    )
    sip_call.update!(message: message)
    sip_call
  end

  describe 'GET /api/v1/accounts/{account.id}/sip_calls/{id}' do
    context 'when unauthenticated' do
      it 'returns unauthorized' do
        sip_call = create_sip_call_with_message

        get "/api/v1/accounts/#{account.id}/sip_calls/#{sip_call.id}"

        expect(response).to have_http_status(:unauthorized)
      end
    end

    context 'when authenticated as agent' do
      it 'returns the sip_call' do
        sip_call = create_sip_call_with_message

        get "/api/v1/accounts/#{account.id}/sip_calls/#{sip_call.id}",
            headers: agent.create_new_auth_token, as: :json

        expect(response).to have_http_status(:success)
        body = response.parsed_body
        expect(body['call_id']).to eq(sip_call.call_id)
        expect(body['status']).to eq('ringing')
        expect(body['direction']).to eq('inbound')
      end

      it 'returns 404 for a sip_call in another account' do
        other_account = create(:account)
        other_inbox = create(:channel_sip, account: other_account).inbox
        other_conversation = create(:conversation, account: other_account, inbox: other_inbox)
        other_call = create(:sip_call, account: other_account, inbox: other_inbox, conversation: other_conversation)

        get "/api/v1/accounts/#{account.id}/sip_calls/#{other_call.id}",
            headers: agent.create_new_auth_token, as: :json

        expect(response).to have_http_status(:not_found)
      end
    end
  end

  describe 'POST /api/v1/accounts/{account.id}/sip_calls/{id}/accept' do
    context 'when unauthenticated' do
      it 'returns unauthorized' do
        sip_call = create_sip_call_with_message

        post "/api/v1/accounts/#{account.id}/sip_calls/#{sip_call.id}/accept"

        expect(response).to have_http_status(:unauthorized)
      end
    end

    context 'when authenticated as agent' do
      it 'accepts the call and returns the updated sip_call' do
        sip_call = create_sip_call_with_message

        post "/api/v1/accounts/#{account.id}/sip_calls/#{sip_call.id}/accept",
             headers: agent.create_new_auth_token,
             params: { sdp_answer: 'answer-sdp' }, as: :json

        expect(response).to have_http_status(:success)
        sip_call.reload
        expect(sip_call.status).to eq('accepted')
      end

      it 'returns 422 when sdp_answer is missing' do
        sip_call = create_sip_call_with_message

        post "/api/v1/accounts/#{account.id}/sip_calls/#{sip_call.id}/accept",
             headers: agent.create_new_auth_token,
             params: {}, as: :json

        expect(response).to have_http_status(:unprocessable_entity)
        expect(response.parsed_body['error']).to eq('sdp_answer is required')
      end

      it 'returns 422 when the call is not ringing' do
        sip_call = create_sip_call_with_message(status: 'accepted')

        post "/api/v1/accounts/#{account.id}/sip_calls/#{sip_call.id}/accept",
             headers: agent.create_new_auth_token,
             params: { sdp_answer: 'answer-sdp' }, as: :json

        expect(response).to have_http_status(:unprocessable_entity)
      end
    end
  end

  describe 'POST /api/v1/accounts/{account.id}/sip_calls/{id}/reject' do
    context 'when unauthenticated' do
      it 'returns unauthorized' do
        sip_call = create_sip_call_with_message

        post "/api/v1/accounts/#{account.id}/sip_calls/#{sip_call.id}/reject"

        expect(response).to have_http_status(:unauthorized)
      end
    end

    context 'when authenticated as agent' do
      it 'rejects the call' do
        sip_call = create_sip_call_with_message

        post "/api/v1/accounts/#{account.id}/sip_calls/#{sip_call.id}/reject",
             headers: agent.create_new_auth_token, as: :json

        expect(response).to have_http_status(:success)
        sip_call.reload
        expect(sip_call.status).to eq('rejected')
        expect(sip_call.end_reason).to eq('agent-rejected')
      end
    end
  end

  describe 'POST /api/v1/accounts/{account.id}/sip_calls/{id}/terminate' do
    context 'when unauthenticated' do
      it 'returns unauthorized' do
        sip_call = create_sip_call_with_message

        post "/api/v1/accounts/#{account.id}/sip_calls/#{sip_call.id}/terminate"

        expect(response).to have_http_status(:unauthorized)
      end
    end

    context 'when authenticated as agent' do
      it 'terminates a ringing call as agent-canceled' do
        sip_call = create_sip_call_with_message

        post "/api/v1/accounts/#{account.id}/sip_calls/#{sip_call.id}/terminate",
             headers: agent.create_new_auth_token, as: :json

        expect(response).to have_http_status(:success)
        sip_call.reload
        expect(sip_call.status).to eq('rejected')
        expect(sip_call.end_reason).to eq('agent-canceled')
      end

      it 'terminates an accepted call as agent-hangup' do
        sip_call = create_sip_call_with_message(status: 'accepted')

        post "/api/v1/accounts/#{account.id}/sip_calls/#{sip_call.id}/terminate",
             headers: agent.create_new_auth_token, as: :json

        expect(response).to have_http_status(:success)
        sip_call.reload
        expect(sip_call.status).to eq('ended')
        expect(sip_call.end_reason).to eq('agent-hangup')
      end
    end
  end

  describe 'POST /api/v1/accounts/{account.id}/sip_calls/initiate' do
    let(:gateway_result) { { 'call_id' => 'outbound-1', 'sdp_answer' => 'gw-answer-sdp' } }

    before do
      allow(gateway_client).to receive(:initiate_call).and_return(gateway_result)
    end

    context 'when unauthenticated' do
      it 'returns unauthorized' do
        post "/api/v1/accounts/#{account.id}/sip_calls/initiate"

        expect(response).to have_http_status(:unauthorized)
      end
    end

    context 'when authenticated as agent' do
      it 'initiates an outbound call and returns the sip_call with sdp_answer' do
        post "/api/v1/accounts/#{account.id}/sip_calls/initiate",
             headers: agent.create_new_auth_token,
             params: { conversation_id: conversation.display_id, sdp_offer: 'offer-sdp' }, as: :json

        expect(response).to have_http_status(:success)
        body = response.parsed_body
        expect(body['call_id']).to eq('outbound-1')
        expect(body['sdp_answer']).to eq('gw-answer-sdp')
      end

      it 'returns 422 when sdp_offer is missing' do
        post "/api/v1/accounts/#{account.id}/sip_calls/initiate",
             headers: agent.create_new_auth_token,
             params: { conversation_id: conversation.display_id }, as: :json

        expect(response).to have_http_status(:unprocessable_entity)
        expect(response.parsed_body['error']).to eq('sdp_offer is required')
      end

      it 'returns 404 when conversation does not exist' do
        post "/api/v1/accounts/#{account.id}/sip_calls/initiate",
             headers: agent.create_new_auth_token,
             params: { conversation_id: 999_999, sdp_offer: 'offer-sdp' }, as: :json

        expect(response).to have_http_status(:not_found)
      end
    end
  end
end
