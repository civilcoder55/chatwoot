class Sip::GatewayClient
  def initialize(channel)
    @gateway_url = channel.gateway_url
    @secret = channel.webhook_secret
  end

  def initiate_call(from:, to:, sdp_offer:)
    post('/calls/initiate', { from: from, to: to, sdp_offer: sdp_offer })
  end

  def accept_call(call_id, sdp_answer)
    post("/calls/#{call_id}/accept", { sdp_answer: sdp_answer })
  end

  def reject_call(call_id)
    post("/calls/#{call_id}/reject", {})
  end

  def terminate_call(call_id)
    post("/calls/#{call_id}/terminate", {})
  end

  private

  def post(path, body)
    response = HTTParty.post(
      "#{@gateway_url}#{path}",
      headers: { 'Content-Type' => 'application/json', 'X-Gateway-Secret' => @secret },
      body: body.to_json,
      timeout: 10
    )

    unless response.success?
      Rails.logger.error "[SIP GATEWAY] #{path} failed: status=#{response.code} body=#{response.body}"
      raise Sip::CallErrors::CallFailed, "Gateway request failed: #{response.code}"
    end

    response.parsed_response
  end
end
