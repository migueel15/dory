package net.dorlab.dory;

/** A dictation engine that records on the device and reports through MoonshineCapture.Listener. */
interface LocalCapture {
    void start();
    void stop();
}
