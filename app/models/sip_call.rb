# == Schema Information
#
# Table name: sip_calls
#
#  id                   :bigint           not null, primary key
#  direction            :string           not null
#  duration_seconds     :integer
#  end_reason           :string
#  meta                 :jsonb            not null
#  status               :string           default("ringing"), not null
#  created_at           :datetime         not null
#  updated_at           :datetime         not null
#  accepted_by_agent_id :bigint
#  account_id           :bigint           not null
#  call_id              :string           not null
#  conversation_id      :bigint           not null
#  inbox_id             :bigint           not null
#  message_id           :bigint
#
# Indexes
#
#  index_sip_calls_on_account_id_and_conversation_id  (account_id,conversation_id)
#  index_sip_calls_on_call_id                         (call_id) UNIQUE
#  index_sip_calls_on_inbox_id_and_status             (inbox_id,status)
#  index_sip_calls_on_message_id                      (message_id)
#
class SipCall < ApplicationRecord
  belongs_to :account
  belongs_to :inbox
  belongs_to :conversation
  belongs_to :accepted_by_agent, class_name: 'User', optional: true
  belongs_to :message, optional: true

  has_one_attached :recording

  validates :call_id, presence: true, uniqueness: true

  enum :status, {
    ringing: 'ringing',
    accepted: 'accepted',
    rejected: 'rejected',
    missed: 'missed',
    ended: 'ended',
    failed: 'failed'
  }
  enum :direction, { inbound: 'inbound', outbound: 'outbound' }

  scope :active, -> { where(status: %w[ringing accepted]) }

  TERMINAL_STATUSES = %w[rejected missed ended failed].freeze

  def terminal?
    TERMINAL_STATUSES.include?(status)
  end

  def self.agent_terminal_reason?(reason)
    Sip::EndReason::AGENT_TERMINAL.include?(reason)
  end

  def self.known_end_reason?(reason)
    Sip::EndReason::STATUS_MAP.key?(reason) || Sip::EndReason::FAILED.include?(reason) || reason.to_s.start_with?('sip-')
  end

  def self.status_for_end_reason(reason)
    Sip::EndReason::STATUS_MAP.fetch(reason, 'failed')
  end

  def sdp_offer
    meta['sdp_offer']
  end

  def ice_servers
    meta['ice_servers'] || []
  end

  def recording_url
    return unless recording.attached?

    Rails.application.routes.url_helpers.rails_blob_path(recording, only_path: true)
  end
end
