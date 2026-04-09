/* global axios */
import ApiClient from './ApiClient';

class SipCallsAPI extends ApiClient {
  constructor() {
    super('sip_calls', { accountScoped: true });
  }

  show(callId) {
    return axios.get(`${this.url}/${callId}`);
  }

  accept(callId, sdpAnswer) {
    return axios.post(`${this.url}/${callId}/accept`, {
      sdp_answer: sdpAnswer,
    });
  }

  reject(callId) {
    return axios.post(`${this.url}/${callId}/reject`);
  }

  terminate(callId) {
    return axios.post(`${this.url}/${callId}/terminate`);
  }

  initiate(conversationId, sdpOffer) {
    return axios.post(`${this.url}/initiate`, {
      conversation_id: conversationId,
      sdp_offer: sdpOffer,
    });
  }
}

export default new SipCallsAPI();
