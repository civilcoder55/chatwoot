class Channel::Sip < ApplicationRecord
  include Channelable

  self.table_name = 'channel_sip'

  validates :phone_number, presence: true, uniqueness: true, format: { with: /\A\+[1-9]\d{1,14}\z/ }
  validates :provider_config, presence: true
  validate :validate_gateway_url

  EDITABLE_ATTRS = [:phone_number, { provider_config: {} }].freeze

  def name
    "SIP (#{phone_number})"
  end

  def messaging_window_enabled?
    false
  end

  def gateway_url
    provider_config['gateway_url']
  end

  def webhook_secret
    provider_config['webhook_secret']
  end

  private

  # [IIHT] check if valid URL or a valid gateway. not all URLs are valid sip gateways
  # we can discuss more on my intention here
  def validate_gateway_url
    return if provider_config.blank?

    errors.add(:provider_config, 'gateway_url is required') if provider_config['gateway_url'].blank?
  end
end
