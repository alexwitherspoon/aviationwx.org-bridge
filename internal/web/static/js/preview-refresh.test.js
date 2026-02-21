/**
 * Unit tests for preview-refresh.js using Node built-in test runner.
 * Run: node --test internal/web/static/js/preview-refresh.test.js
 */
import test from 'node:test';
import assert from 'node:assert';
import { shouldRefreshPreview } from './preview-refresh.js';

function statusWithCapture(cameraId, lastCaptureTime) {
    return {
        orchestrator: {
            camera_stats: [
                {
                    camera_id: cameraId,
                    capture_stats: {
                        last_capture_time: lastCaptureTime,
                    },
                },
            ],
        },
    };
}

test('shouldRefreshPreview returns false when no status', () => {
    const map = new Map();
    assert.strictEqual(shouldRefreshPreview('cam1', map, null), false);
    assert.strictEqual(shouldRefreshPreview('cam1', map, undefined), false);
});

test('shouldRefreshPreview returns false when no orchestrator', () => {
    const map = new Map();
    assert.strictEqual(shouldRefreshPreview('cam1', map, {}), false);
});

test('shouldRefreshPreview returns false when no camera stats', () => {
    const map = new Map();
    assert.strictEqual(shouldRefreshPreview('cam1', map, { orchestrator: {} }), false);
});

test('shouldRefreshPreview returns false when camera not in stats', () => {
    const map = new Map();
    const status = statusWithCapture('other-cam', '2026-02-20T12:00:00Z');
    assert.strictEqual(shouldRefreshPreview('cam1', map, status), false);
});

test('shouldRefreshPreview returns false when no last_capture_time', () => {
    const map = new Map();
    const status = {
        orchestrator: {
            camera_stats: [{ camera_id: 'cam1', capture_stats: {} }],
        },
    };
    assert.strictEqual(shouldRefreshPreview('cam1', map, status), false);
});

test('shouldRefreshPreview returns true when never displayed (first load)', () => {
    const map = new Map();
    const status = statusWithCapture('cam1', '2026-02-20T12:00:00Z');
    assert.strictEqual(shouldRefreshPreview('cam1', map, status), true);
});

test('shouldRefreshPreview returns true when new capture is newer', () => {
    const map = new Map();
    map.set('cam1', new Date('2026-02-20T12:00:00Z').getTime());
    const status = statusWithCapture('cam1', '2026-02-20T12:01:00Z');
    assert.strictEqual(shouldRefreshPreview('cam1', map, status), true);
});

test('shouldRefreshPreview returns false when last displayed is same or newer', () => {
    const map = new Map();
    const captureTime = new Date('2026-02-20T12:00:00Z').getTime();
    map.set('cam1', captureTime);
    const status = statusWithCapture('cam1', '2026-02-20T12:00:00Z');
    assert.strictEqual(shouldRefreshPreview('cam1', map, status), false);
});

test('shouldRefreshPreview returns false when last displayed is newer', () => {
    const map = new Map();
    map.set('cam1', new Date('2026-02-20T12:01:00Z').getTime());
    const status = statusWithCapture('cam1', '2026-02-20T12:00:00Z');
    assert.strictEqual(shouldRefreshPreview('cam1', map, status), false);
});

test('shouldRefreshPreview handles multiple cameras independently', () => {
    const map = new Map();
    map.set('cam1', new Date('2026-02-20T12:00:00Z').getTime());
    const status = {
        orchestrator: {
            camera_stats: [
                {
                    camera_id: 'cam1',
                    capture_stats: { last_capture_time: '2026-02-20T12:00:00Z' },
                },
                {
                    camera_id: 'cam2',
                    capture_stats: { last_capture_time: '2026-02-20T12:05:00Z' },
                },
            ],
        },
    };
    assert.strictEqual(shouldRefreshPreview('cam1', map, status), false);
    assert.strictEqual(shouldRefreshPreview('cam2', map, status), true);
});
