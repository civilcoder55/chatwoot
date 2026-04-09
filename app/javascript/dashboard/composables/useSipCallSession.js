import { ref, computed, watch, onUnmounted } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  useSipCallsStore,
  getSipOutboundCallState,
} from 'dashboard/stores/sipCalls';
import SipCallsAPI from 'dashboard/api/sipCalls';
import Auth from 'dashboard/api/auth';
import Timer from 'dashboard/helper/Timer';

let inboundPc = null;
let inboundStream = null;
let inboundAudio = null;

function cleanupInboundWebRTC() {
  if (inboundStream) {
    inboundStream.getTracks().forEach(track => track.stop());
    inboundStream = null;
  }
  if (inboundPc) {
    inboundPc.close();
    inboundPc = null;
  }
  if (inboundAudio) {
    inboundAudio.srcObject = null;
    if (inboundAudio.parentNode) {
      inboundAudio.parentNode.removeChild(inboundAudio);
    }
    inboundAudio = null;
  }
}

function waitForIceGatheringComplete(pc) {
  return new Promise((resolve, reject) => {
    if (pc.iceGatheringState === 'complete') {
      resolve();
      return;
    }
    const timeout = setTimeout(() => {
      // eslint-disable-next-line no-console
      console.warn('[SIP Call] ICE gathering timed out, sending partial SDP');
      resolve();
    }, 10000);

    pc.onicegatheringstatechange = () => {
      if (pc.iceGatheringState === 'complete') {
        clearTimeout(timeout);
        resolve();
      }
    };
    pc.oniceconnectionstatechange = () => {
      if (pc.iceConnectionState === 'failed') {
        clearTimeout(timeout);
        reject(new Error('ICE connection failed'));
      }
    };
  });
}

async function doAcceptCall(call) {
  cleanupInboundWebRTC();

  const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
  inboundStream = stream;

  // const iceServers = call.iceServers?.length
  //   ? call.iceServers
  //   : [{ urls: 'stun:stun.l.google.com:19302' }];

  const pc = new RTCPeerConnection();
  inboundPc = pc;

  stream.getTracks().forEach(track => pc.addTrack(track, stream));

  pc.ontrack = event => {
    const [remoteStream] = event.streams;
    if (!remoteStream) return;
    if (!inboundAudio) {
      const audio = document.createElement('audio');
      audio.autoplay = true;
      document.body.appendChild(audio);
      inboundAudio = audio;
    }
    inboundAudio.srcObject = remoteStream;
    inboundAudio.play().catch(() => {});
  };

  await pc.setRemoteDescription({ type: 'offer', sdp: call.sdpOffer });
  const answer = await pc.createAnswer();
  await pc.setLocalDescription(answer);
  await waitForIceGatheringComplete(pc);

  const completeSdp = pc.localDescription.sdp;
  await SipCallsAPI.accept(call.id, completeSdp);

  return { success: true };
}

export async function acceptSipCallById(sipCallId) {
  const callsStore = useSipCallsStore();

  if (callsStore.hasActiveCall) {
    return { success: false, error: 'active_call_exists' };
  }

  let call = callsStore.incomingCalls.find(
    c => c.id === sipCallId || c.sipCallId === sipCallId
  );

  if (!call) {
    const { data } = await SipCallsAPI.show(sipCallId);
    if (data.status !== 'ringing') {
      return { success: false, error: 'not_ringing' };
    }
    call = {
      id: data.id,
      callId: data.call_id,
      sipCallId: data.id,
      direction: data.direction,
      inboxId: data.inbox_id,
      conversationId: data.conversation_id,
      sdpOffer: data.sdp_offer,
      iceServers: data.ice_servers,
      caller: data.caller,
    };
    callsStore.addIncomingCall(call);
  }

  await doAcceptCall(call);
  callsStore.removeIncomingCall(call.callId);
  callsStore.setActiveCall({ ...call });

  return { success: true, call };
}

function terminateCallOnUnload(callId) {
  const authData = Auth.hasAuthCookie() ? Auth.getAuthData() : {};
  const accountId =
    window.location.pathname.includes('/app/accounts') &&
    window.location.pathname.split('/')[3];
  if (!accountId) return;

  const url = `/api/v1/accounts/${accountId}/sip_calls/${callId}/terminate`;
  fetch(url, {
    method: 'POST',
    keepalive: true,
    headers: {
      'Content-Type': 'application/json',
      'access-token': authData['access-token'] || '',
      'token-type': authData['token-type'] || '',
      client: authData.client || '',
      expiry: authData.expiry || '',
      uid: authData.uid || '',
    },
  }).catch(() => {});
}

export function useSipCallSession() {
  const { t } = useI18n();
  const callsStore = useSipCallsStore();

  const isAccepting = ref(false);
  const isMuted = ref(false);
  const callError = ref(null);
  const callDuration = ref(0);

  const durationTimer = new Timer(elapsed => {
    callDuration.value = elapsed;
  });

  const activeCall = computed(() => callsStore.activeCall);
  const incomingCalls = computed(() => callsStore.incomingCalls);
  const hasActiveCall = computed(() => callsStore.hasActiveCall);
  const hasIncomingCall = computed(() => callsStore.hasIncomingCall);
  const firstIncomingCall = computed(() => callsStore.firstIncomingCall);

  // "Calling..." — outbound call initiated, waiting for remote to ring
  const isOutboundCalling = computed(
    () =>
      activeCall.value?.direction === 'outbound' &&
      activeCall.value?.status !== 'connected' &&
      activeCall.value?.status !== 'ringing'
  );

  // "Ringing..." — remote phone is actually ringing (SIP 180 received)
  const isOutboundRinging = computed(
    () =>
      activeCall.value?.direction === 'outbound' &&
      activeCall.value?.status === 'ringing'
  );

  // Either calling or ringing — call not yet connected
  const isOutboundPending = computed(
    () => isOutboundCalling.value || isOutboundRinging.value
  );

  const formattedCallDuration = computed(() => {
    const minutes = Math.floor(callDuration.value / 60);
    const seconds = callDuration.value % 60;
    return `${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`;
  });

  callsStore.registerCleanupCallback(() => {
    cleanupInboundWebRTC();
    durationTimer.stop();
    callDuration.value = 0;
  });

  const handleBeforeUnload = () => {
    const call = callsStore.activeCall;
    if (call?.id) {
      terminateCallOnUnload(call.id);
      cleanupInboundWebRTC();
    }
  };
  window.addEventListener('beforeunload', handleBeforeUnload);

  watch(activeCall, call => {
    if (
      call?.direction === 'outbound' &&
      call?.status === 'connected' &&
      !durationTimer.intervalId
    ) {
      durationTimer.start();
    }
  });

  const acceptCall = async call => {
    if (isAccepting.value) return;
    isAccepting.value = true;
    callError.value = null;

    try {
      await doAcceptCall(call);
      callsStore.removeIncomingCall(call.callId);
      callsStore.setActiveCall({ ...call });
      durationTimer.start();
    } catch (err) {
      callError.value =
        err.name === 'NotAllowedError'
          ? t('SIP_CALL.MIC_DENIED')
          : t('SIP_CALL.CALL_FAILED');
      cleanupInboundWebRTC();
    } finally {
      isAccepting.value = false;
    }
  };

  const rejectCall = async call => {
    try {
      await SipCallsAPI.reject(call.id);
    } catch {
      // Best effort
    } finally {
      callsStore.removeIncomingCall(call.callId);
    }
  };

  const endActiveCall = async () => {
    const call = activeCall.value;
    if (!call) return;

    try {
      await SipCallsAPI.terminate(call.id);
    } catch {
      // Best effort
    } finally {
      cleanupInboundWebRTC();
      callsStore.handleCallEnded(call.callId);
      callsStore.clearActiveCall();
      durationTimer.stop();
      callDuration.value = 0;
    }
  };

  const toggleMute = () => {
    const stream = inboundStream || getSipOutboundCallState().stream;
    if (!stream) return;
    const audioTrack = stream.getAudioTracks()[0];
    if (!audioTrack) return;
    audioTrack.enabled = !audioTrack.enabled;
    isMuted.value = !audioTrack.enabled;
  };

  const dismissIncomingCall = call => {
    callsStore.removeIncomingCall(call.callId);
  };

  onUnmounted(() => {
    window.removeEventListener('beforeunload', handleBeforeUnload);
    durationTimer.stop();
  });

  return {
    activeCall,
    incomingCalls,
    hasActiveCall,
    hasIncomingCall,
    firstIncomingCall,
    isAccepting,
    isMuted,
    isOutboundCalling,
    isOutboundRinging,
    isOutboundPending,
    callError,
    formattedCallDuration,
    acceptCall,
    rejectCall,
    endActiveCall,
    toggleMute,
    dismissIncomingCall,
  };
}
