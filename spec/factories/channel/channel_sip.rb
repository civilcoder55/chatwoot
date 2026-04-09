# frozen_string_literal: true

FactoryBot.define do
  factory :channel_sip, class: 'Channel::Sip' do
    sequence(:phone_number) { |n| "+155512390#{n.to_s.rjust(2, '0')}" }
    provider_config do
      {
        gateway_url: 'http://sip-gateway.test',
        webhook_secret: SecureRandom.hex(16)
      }
    end
    account

    after(:create) do |channel_sip|
      create(:inbox, channel: channel_sip, account: channel_sip.account)
    end
  end
end
