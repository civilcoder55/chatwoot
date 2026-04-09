class Channel::Sip < ApplicationRecord
  include Channelable

  self.table_name = 'channel_sip'

  before_validation :ensure_valid_tenant, on: :create
  validates :phone_number, presence: true, uniqueness: true, format: { with: /\A\+[1-9]\d{1,14}\z/ }
  validates :provider_config, presence: true

  EDITABLE_ATTRS = [:phone_number, { provider_config: {} }].freeze

  def name
    "SIP (#{phone_number})"
  end

  def messaging_window_enabled?
    false
  end

  def api_key
    provider_config['api_key']
  end

  private

  def ensure_valid_tenant
    return unless phone_number.present? && api_key.present?

    unless Sip::GatewayClient.new(self).validate_tenant
      errors.add(:base, 'invalid tenant credentials')
    end
  end
end
