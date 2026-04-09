# [IIHT]: Add HMAC signature verification for webhook security.
# standard webhook and public api security practices
class Webhooks::SipGatewayController < ActionController::API
  before_action :authenticate_gateway!

  def events
    event = params[:event] || params.dig(:data, :event)
    data = params[:data] || params

    Sip::WebhookEventJob.perform_later(data.to_unsafe_h.merge(event: event))
    head :ok
  end

  def recordings
    validate_recording_params!

    sip_call = SipCall.find_by!(call_id: params[:call_id])

    Sip::RecordingAttachmentService.attach_uploaded_recording!(
      sip_call: sip_call,
      recording: params[:recording]
    )

    render json: { id: sip_call.id, status: 'uploaded', recording_url: sip_call.recording_url }
  rescue StandardError => e
    Rails.logger.error "[SIP CALL] recording upload failed: #{e.message}"
    render json: { error: 'Failed to upload recording' }, status: :internal_server_error
  end

  private

  def authenticate_gateway!
    gateway_secret = request.headers['X-Gateway-Secret'].to_s
    expected_secret = ENV.fetch('SIP_GATEWAY_SECRET', 'test')

    return if gateway_secret.bytesize == expected_secret.bytesize &&
              ActiveSupport::SecurityUtils.secure_compare(gateway_secret, expected_secret)

    head :unauthorized
  end

  def validate_recording_params!
    return render json: { error: 'call_id is required' }, status: :unprocessable_entity if params[:call_id].blank?
    return render json: { error: 'recording is required' }, status: :unprocessable_entity if params[:recording].blank?
  end
end
