class Sip::CallTimeoutJob < ApplicationJob
  queue_as :default

  def perform(sip_call_id)
    sip_call = SipCall.find_by(id: sip_call_id)
    return unless sip_call&.ringing?

    Sip::IncomingCallService.new(
      inbox: sip_call.inbox,
      params: { event: Sip::IncomingCallService::WEBHOOK_ENDED, call_id: sip_call.call_id, reason: Sip::ConversationCallStatus::NO_ANSWER }
    ).perform
  end
end
