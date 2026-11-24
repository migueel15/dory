export async function openDictationMicrophone(): Promise<MediaStream> {
  try {
    return await navigator.mediaDevices.getUserMedia({ audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true } });
  } catch (error) {
    // Some Android audio devices cannot start Chromium's processed input.
    if (!(error instanceof DOMException) || error.name !== "NotReadableError") throw error;
    return navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: false, noiseSuppression: false, autoGainControl: false } });
  }
}
