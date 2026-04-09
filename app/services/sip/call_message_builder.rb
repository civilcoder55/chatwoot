class Sip::CallMessageBuilder
  def self.create!(conversation:, sip_call:, user: nil)
    new(conversation: conversation, sip_call: sip_call, user: user).create!
  end

  def self.update_status!(sip_call:, status: nil, agent: nil, duration_seconds: nil)
    new(conversation: sip_call.conversation, sip_call: sip_call).update_status!(
      status: status, agent: agent, duration_seconds: duration_seconds
    )
  end

  def self.update_recording_url!(sip_call:)
    message = sip_call.message
    return unless message

    data = (message.content_attributes || {}).dup
    data['data'] ||= {}
    data['data']['recording_url'] = sip_call.recording_url
    message.update!(content_attributes: data)
  end

  def initialize(conversation:, sip_call:, user: nil)
    @conversation = conversation
    @sip_call = sip_call
    @user = user
  end

  def create!
    params = {
      content: 'SIP Call',
      message_type: message_type,
      content_type: 'voice_call',
      content_attributes: { 'data' => build_data_payload }
    }

    Messages::MessageBuilder.new(sender, conversation, params).perform
  end

  def update_status!(status:, agent: nil, duration_seconds: nil)
    message = sip_call.message
    return unless message

    data = (message.content_attributes || {}).dup
    data['data'] ||= {}
    data['data']['status'] = map_status(status) if status
    data['data']['accepted_by'] = { 'id' => agent.id, 'name' => agent.name } if agent
    data['data']['duration_seconds'] = duration_seconds if duration_seconds

    message.update!(content_attributes: data)
    message
  end

  private

  attr_reader :conversation, :sip_call, :user

  def build_data_payload
    {
      'call_sid' => sip_call.call_id,
      'status' => map_status(sip_call.status),
      'call_direction' => sip_call.direction,
      'call_source' => 'sip',
      'sip_call_id' => sip_call.id,
      'from_number' => from_number,
      'to_number' => to_number,
      'meta' => { 'created_at' => Time.zone.now.to_i }
    }
  end

  def message_type
    sip_call.direction == 'outbound' ? 'outgoing' : 'incoming'
  end

  def sender
    return user if sip_call.direction == 'outbound' && user

    conversation.contact
  end

  def from_number
    if sip_call.direction == 'inbound'
      conversation.contact&.phone_number
    else
      conversation.inbox.channel&.phone_number
    end
  end

  def to_number
    if sip_call.direction == 'inbound'
      conversation.inbox.channel&.phone_number
    else
      conversation.contact&.phone_number
    end
  end

  def map_status(status)
    Sip::VoiceStatus::SIP_TO_VOICE[status] || status
  end
end
