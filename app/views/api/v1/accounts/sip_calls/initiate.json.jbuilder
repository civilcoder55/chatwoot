json.partial! 'api/v1/models/sip_call', formats: [:json], sip_call: @sip_call
json.sdp_answer @sip_call.meta['sdp_answer']
