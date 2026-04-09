class Sip::ActivityMessageService
  AGENT_ACTION_KEYS = {
    answered: 'agent_answered',
    rejected: 'agent_rejected',
    canceled: 'agent_canceled',
    ended: 'agent_hangup'
  }.freeze

  TERMINAL_REASON_KEYS = {
    'remote-canceled' => 'remote_canceled',
    'remote-hangup' => 'remote_hangup',
    'busy' => 'busy',
    'rejected' => 'remote_rejected',
    'no-answer' => 'no_answer',
    'not-found' => 'not_found',
    'service-unavailable' => 'service_unavailable'
  }.freeze

  REMOTE_PARTY_KEYS = %w[remote_canceled remote_hangup busy remote_rejected not_found].freeze

  pattr_initialize [:conversation!, :action!, :direction!, { user: nil, reason: nil, call_id: nil }]

  def perform
    content = formatted_activity_content
    return unless content

    Conversations::ActivityMessageJob.perform_later(conversation, activity_message_params(content))
  end

  private

  def formatted_activity_content
    content = activity_content
    return unless content
    return content if call_id.blank?

    "#{content} (#{translate('call_id', call_id: call_id)})"
  end

  def activity_content
    return terminal_activity_content if action.to_sym == :terminal && validate_terminal_reason

    return unless user

    key = AGENT_ACTION_KEYS[action.to_sym]
    return unless key

    translate(key, user_name: user.name)
  end

  def terminal_activity_content
    key = TERMINAL_REASON_KEYS[reason] || 'failed'
    translate(key, terminal_translation_options(key))
  end

  def validate_terminal_reason
    TERMINAL_REASON_KEYS[reason].present? || reason.to_s.start_with?('sip-')
  end

  def terminal_translation_options(key)
    return {} unless REMOTE_PARTY_KEYS.include?(key)

    { party: remote_party_label }
  end

  def remote_party_label
    translate(direction == 'inbound' ? 'party_caller' : 'party_recipient')
  end

  def translate(key, values = {})
    I18n.t("conversations.activity.sip_call.#{key}", **values, locale: locale)
  end

  def locale
    conversation.account.locale
  end

  def activity_message_params(content)
    {
      account_id: conversation.account_id,
      inbox_id: conversation.inbox_id,
      message_type: :activity,
      content: content
    }
  end
end
