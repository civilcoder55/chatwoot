module Sip::VoiceStatus
  SIP_TO_VOICE = {
    'ringing' => Sip::ConversationCallStatus::RINGING,
    'accepted' => Sip::ConversationCallStatus::IN_PROGRESS,
    'rejected' => Sip::ConversationCallStatus::FAILED,
    'missed' => Sip::ConversationCallStatus::NO_ANSWER,
    'ended' => Sip::ConversationCallStatus::COMPLETED,
    'failed' => Sip::ConversationCallStatus::FAILED
  }.freeze
end
