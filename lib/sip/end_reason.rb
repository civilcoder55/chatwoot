module Sip::EndReason
  AGENT_REJECTED  = 'agent-rejected'.freeze
  AGENT_CANCELED  = 'agent-canceled'.freeze
  AGENT_HANGUP    = 'agent-hangup'.freeze
  REMOTE_CANCELED = 'remote-canceled'.freeze
  REMOTE_HANGUP   = 'remote-hangup'.freeze
  BUSY            = 'busy'.freeze
  REJECTED        = 'rejected'.freeze
  NO_ANSWER       = 'no-answer'.freeze
  FAILED_STATUS   = 'failed'.freeze

  CALLER_HANGUP  = 'caller-hangup'.freeze
  GATEWAY_HANGUP = 'gateway-hangup'.freeze

  AGENT_TERMINAL = [AGENT_REJECTED, AGENT_CANCELED, AGENT_HANGUP].freeze

  FAILED = %w[
    webrtc-setup-failed
    sip-invite-failed
    sip-setup-failed
    sip-accept-failed
    sip-ack-failed
    service-unavailable
    not-found
  ].freeze

  STATUS_MAP = {
    AGENT_REJECTED => 'rejected'.freeze,
    AGENT_CANCELED => 'rejected'.freeze,
    BUSY => 'rejected'.freeze,
    REJECTED => 'rejected'.freeze,
    REMOTE_CANCELED => 'missed'.freeze,
    NO_ANSWER => 'missed'.freeze,
    AGENT_HANGUP => 'ended'.freeze,
    REMOTE_HANGUP => 'ended'.freeze
  }.freeze
end
