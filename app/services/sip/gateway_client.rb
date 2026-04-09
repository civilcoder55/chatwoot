class Sip::GatewayClient
  def initialize(channel)
    @api_key = channel.api_key
    @phone_number = channel.phone_number
  end

  def validate_tenant
    response = post('/validate', {})
    response['valid'] == true
  rescue StandardError
    false
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
    url = ENV.fetch('SIP_GATEWAY_URL', 'http://gateway.local')
    raise StandardError, 'SIP_GATEWAY_URL not configured' if url.blank?

    response = HTTParty.post(
      "#{url}#{path}",
      headers: { 'Content-Type' => 'application/json', 'X-Api-Key' => @api_key, 'X-Phone-Number' => @phone_number },
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
