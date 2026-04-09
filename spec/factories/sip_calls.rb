# frozen_string_literal: true

FactoryBot.define do
  factory :sip_call do
    account
    inbox { create(:channel_sip, account: account).inbox }
    conversation { create(:conversation, account: account, inbox: inbox) }
    sequence(:call_id) { |n| "sip-call-#{n}" }
    direction { 'inbound' }
    status { 'ringing' }
    meta { { 'sdp_offer' => 'offer-sdp', 'ice_servers' => [{ 'urls' => 'stun:stun.l.google.com:19302' }] } }
  end
end
