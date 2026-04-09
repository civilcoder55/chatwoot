# == Schema Information
#
# Table name: whatsapp_calls
#
#  id                   :bigint           not null, primary key
#  direction            :string           not null
#  duration_seconds     :integer
#  end_reason           :string
#  meta                 :jsonb            not null
#  status               :string           default("ringing"), not null
#  transcript           :text
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
#  index_whatsapp_calls_on_account_id_and_conversation_id  (account_id,conversation_id)
#  index_whatsapp_calls_on_call_id                         (call_id) UNIQUE
#  index_whatsapp_calls_on_inbox_id_and_status             (inbox_id,status)
#  index_whatsapp_calls_on_message_id                      (message_id)
#
# Foreign Keys
#
#  fk_rails_...  (message_id => messages.id)
#
class WhatsappCall < ApplicationRecord
  STATUSES = %w[ringing accepted rejected missed ended failed].freeze
  DIRECTIONS = %w[inbound outbound].freeze

  belongs_to :account
  belongs_to :inbox
  belongs_to :conversation
  belongs_to :accepted_by_agent, class_name: 'User', optional: true
  belongs_to :message, optional: true

  has_one_attached :recording

  validates :call_id, presence: true, uniqueness: true
  validates :direction, inclusion: { in: DIRECTIONS }
  validates :status, inclusion: { in: STATUSES }

  scope :active, -> { where(status: %w[ringing accepted]) }
  scope :ringing, -> { where(status: 'ringing') }

  def accepted?
    status == 'accepted'
  end

  def ringing?
    status == 'ringing'
  end

  def terminal?
    %w[rejected missed ended failed].include?(status)
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
