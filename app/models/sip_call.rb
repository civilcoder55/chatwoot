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
