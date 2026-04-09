class Sip::WebhookEventJob < ApplicationJob
  queue_as :default

  def perform(params)
    params = params.with_indifferent_access
    inbox = resolve_inbox(params)
    return Rails.logger.warn "[SIP CALL] No inbox found for phone_number: #{params[:phone_number]}" unless inbox

    Sip::IncomingCallService.new(inbox: inbox, params: params).perform
  end

  private

  def resolve_inbox(params)
    phone_number = params[:phone_number]
    return if phone_number.blank?

    normalized = phone_number.start_with?('+') ? phone_number : "+#{phone_number}"
    Channel::Sip.find_by(phone_number: normalized)&.inbox
  end
end
