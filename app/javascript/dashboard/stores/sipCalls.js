import { defineStore } from 'pinia';

const outboundCall = { pc: null, stream: null, audio: null, callId: null };

export function getSipOutboundCallState() {
  return outboundCall;
}

export function setSipOutboundCallProperty(key, value) {
  outboundCall[key] = value;
}

function cleanupOutboundCall() {
  if (outboundCall.pc) outboundCall.pc.close();
  if (outboundCall.stream) {
    outboundCall.stream.getTracks().forEach(t => t.stop());
  }
  if (outboundCall.audio) {
    outboundCall.audio.srcObject = null;
    outboundCall.audio.remove();
  }
  outboundCall.pc = null;
  outboundCall.stream = null;
  outboundCall.audio = null;
  outboundCall.callId = null;
}

export const useSipCallsStore = defineStore('sipCalls', {
  state: () => ({
    incomingCalls: [],
    activeCall: null,
    cleanupCallback: null,
  }),

  getters: {
    hasIncomingCall: state => state.incomingCalls.length > 0,
    hasActiveCall: state => state.activeCall !== null,
    hasSipCall: state =>
      state.incomingCalls.length > 0 || state.activeCall !== null,
    firstIncomingCall: state => state.incomingCalls[0] || null,
  },

  actions: {
    addIncomingCall(callData) {
      const exists = this.incomingCalls.some(c => c.callId === callData.callId);
      if (exists) return;
      this.incomingCalls.push(callData);
    },

    removeIncomingCall(callId) {
      this.incomingCalls = this.incomingCalls.filter(c => c.callId !== callId);
    },

    setActiveCall(callData) {
      this.activeCall = callData;
    },

    clearActiveCall() {
      this.activeCall = null;
    },

    markOutboundRinging(callId) {
      if (
        this.activeCall?.callId === callId &&
        this.activeCall?.status !== 'connected'
      ) {
        this.activeCall = { ...this.activeCall, status: 'ringing' };
      }
    },

    markActiveCallConnected() {
      if (this.activeCall) {
        this.activeCall = { ...this.activeCall, status: 'connected' };
      }
    },

    registerCleanupCallback(callback) {
      this.cleanupCallback = callback;
    },

    handleCallAcceptedByOther(callId) {
      this.removeIncomingCall(callId);
    },

    handleCallEnded(callId) {
      this.removeIncomingCall(callId);
      if (this.activeCall?.callId === callId) {
        this.activeCall = null;
        if (this.cleanupCallback) {
          this.cleanupCallback();
        }
      }
      if (outboundCall.callId === callId) {
        cleanupOutboundCall();
      }
    },
  },
});
