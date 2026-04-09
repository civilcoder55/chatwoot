json.id sip_call.id
json.account_id sip_call.account_id
json.conversation_id sip_call.conversation.display_id
json.call_id sip_call.call_id
json.status sip_call.status
json.direction sip_call.direction
json.inbox_id sip_call.inbox_id
json.message_id sip_call.message_id
json.sdp_offer sip_call.ringing? ? sip_call.sdp_offer : nil
json.ice_servers sip_call.ice_servers
json.caller do
  if sip_call.conversation&.contact.present?
    contact = sip_call.conversation.contact
    json.name contact.name
    json.phone contact.phone_number
    json.avatar contact.avatar_url
  end
end
