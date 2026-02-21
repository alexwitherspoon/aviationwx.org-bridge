/**
 * Pure logic for when to refresh camera preview images. Testable without DOM.
 * Preview should only update when a new capture exists (last_capture_time changed).
 */

/**
 * shouldRefreshPreview returns true when the preview should be refreshed because
 * a new capture exists (last_capture_time is newer than last displayed).
 * @param {string} cameraId - Camera ID
 * @param {Map<string,number>} lastDisplayedMap - Map of cameraId -> last displayed capture timestamp (ms)
 * @param {Object} [status] - Status object with orchestrator.camera_stats
 * @returns {boolean} True if preview should refresh
 */
export function shouldRefreshPreview(cameraId, lastDisplayedMap, status) {
    const camStats = status?.orchestrator?.camera_stats?.find((cs) => cs.camera_id === cameraId);
    const lastCapture = camStats?.capture_stats?.last_capture_time;
    if (!lastCapture) return false;

    const lastDisplayed = lastDisplayedMap?.get(cameraId);
    const newCaptureTime = new Date(lastCapture).getTime();
    if (lastDisplayed !== undefined && lastDisplayed >= newCaptureTime) {
        return false;
    }
    return true;
}
