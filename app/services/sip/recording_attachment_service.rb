class Sip::RecordingAttachmentService
  def self.attach_uploaded_recording!(sip_call:, recording:)
    new(sip_call: sip_call).attach_uploaded_recording!(recording)
  end

  def initialize(sip_call:)
    @sip_call = sip_call
  end

  def attach_uploaded_recording!(recording)
    recording.tempfile.rewind if recording.respond_to?(:tempfile)
    sip_call.recording.attach(
      io: recording.tempfile,
      filename: recording.original_filename.presence || "#{sip_call.call_id}.wav",
      content_type: normalized_content_type(recording.content_type)
    )
    refresh_recording_url!
  end

  private

  attr_reader :sip_call

  def refresh_recording_url!
    Sip::CallMessageBuilder.update_recording_url!(sip_call: sip_call)
    true
  end

  def normalized_content_type(content_type)
    return 'audio/wav' if content_type.blank? || content_type == 'application/octet-stream'

    content_type
  end
end
