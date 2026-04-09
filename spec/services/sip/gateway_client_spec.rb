require 'rails_helper'

RSpec.describe Sip::GatewayClient, type: :service do
  let(:channel) do
    instance_double(Channel::Sip, api_key: 'secret-token', phone_number: '+15550001111')
  end
  let(:client) { described_class.new(channel) }
  ENV['SIP_GATEWAY_URL'] = 'http://sip-gateway.test'
  describe '#initiate_call' do
    it 'posts to /calls/initiate and returns parsed response' do
      stub_request(:post, 'http://sip-gateway.test/calls/initiate')
        .with(
          body: { from: '+15550001111', to: '+15550002222', sdp_offer: 'offer-sdp' }.to_json,
          headers: { 'Content-Type' => 'application/json', 'X-Api-Key' => 'secret-token', 'X-Phone-Number' => '+15550001111' }
        )
        .to_return(status: 200, body: { call_id: 'call-123', sdp_answer: 'answer-sdp' }.to_json, headers: { 'Content-Type' => 'application/json' })

      result = client.initiate_call(from: '+15550001111', to: '+15550002222', sdp_offer: 'offer-sdp')

      expect(result['call_id']).to eq('call-123')
      expect(result['sdp_answer']).to eq('answer-sdp')
    end

    it 'raises CallFailed when gateway returns non-success status' do
      stub_request(:post, 'http://sip-gateway.test/calls/initiate')
        .to_return(status: 500, body: 'Internal Server Error')

      expect do
        client.initiate_call(from: '+15550001111', to: '+15550002222', sdp_offer: 'offer-sdp')
      end.to raise_error(Sip::CallErrors::CallFailed, /Gateway request failed: 500/)
    end
  end

  describe '#accept_call' do
    it 'posts to /calls/:call_id/accept with sdp_answer' do
      stub_request(:post, 'http://sip-gateway.test/calls/call-123/accept')
        .with(
          body: { sdp_answer: 'answer-sdp' }.to_json,
          headers: { 'Content-Type' => 'application/json', 'X-Api-Key' => 'secret-token', 'X-Phone-Number' => '+15550001111' }
        )
        .to_return(status: 200, body: { ok: true }.to_json, headers: { 'Content-Type' => 'application/json' })

      result = client.accept_call('call-123', 'answer-sdp')
      expect(result['ok']).to be(true)
    end

    it 'raises CallFailed on 4xx errors' do
      stub_request(:post, 'http://sip-gateway.test/calls/call-123/accept')
        .to_return(status: 404, body: 'Not Found')

      expect do
        client.accept_call('call-123', 'answer-sdp')
      end.to raise_error(Sip::CallErrors::CallFailed, /Gateway request failed: 404/)
    end
  end

  describe '#reject_call' do
    it 'posts to /calls/:call_id/reject' do
      stub_request(:post, 'http://sip-gateway.test/calls/call-123/reject')
        .with(
          body: {}.to_json,
          headers: { 'Content-Type' => 'application/json', 'X-Api-Key' => 'secret-token', 'X-Phone-Number' => '+15550001111' }
        )
        .to_return(status: 200, body: { ok: true }.to_json, headers: { 'Content-Type' => 'application/json' })

      result = client.reject_call('call-123')
      expect(result['ok']).to be(true)
    end
  end

  describe '#terminate_call' do
    it 'posts to /calls/:call_id/terminate' do
      stub_request(:post, 'http://sip-gateway.test/calls/call-123/terminate')
        .with(
          body: {}.to_json,
          headers: { 'Content-Type' => 'application/json', 'X-Api-Key' => 'secret-token', 'X-Phone-Number' => '+15550001111' }
        )
        .to_return(status: 200, body: { ok: true }.to_json, headers: { 'Content-Type' => 'application/json' })

      result = client.terminate_call('call-123')
      expect(result['ok']).to be(true)
    end

    it 'raises CallFailed on gateway error' do
      stub_request(:post, 'http://sip-gateway.test/calls/call-123/terminate')
        .to_return(status: 503, body: 'Service Unavailable')

      expect do
        client.terminate_call('call-123')
      end.to raise_error(Sip::CallErrors::CallFailed, /Gateway request failed: 503/)
    end
  end
end
