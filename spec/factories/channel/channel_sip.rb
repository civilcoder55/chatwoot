# frozen_string_literal: true

FactoryBot.define do
  factory :channel_sip, class: 'Channel::Sip' do
    sequence(:phone_number) { |n| "+155512390#{n.to_s.rjust(2, '0')}" }
    provider_config do
      {
        api_key: SecureRandom.hex(16)
      }
    end
    account

    before(:create) do |channel_sip|
      channel_sip.define_singleton_method(:ensure_valid_tenant) { nil }
    end

    after(:create) do |channel_sip|
      create(:inbox, channel: channel_sip, account: channel_sip.account)
    end
  end
end
