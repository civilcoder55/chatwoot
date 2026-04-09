module Sip::CallErrors
  class NotRinging < StandardError; end
  class AlreadyAccepted < StandardError; end
  class CallFailed < StandardError; end
end
