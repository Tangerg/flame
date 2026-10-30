# Completion notifications

This optional client package supplies a `clientEffect` for completion policy and
an installed Notifications settings pane. It has no backend. The Host alone
produces settled root-run facts and owns OS permission, actual delivery and
notification-click navigation. The package chooses focus policy, copy and sound.

Both components use the package's one preference boundary. It retains the current
`flame.completion-sound` and `flame.system-notifications` records, including their
`{state: {<name>: boolean}, version: 0}` encoding. Sound defaults off and system
notifications default on. Missing or malformed saved values retain the previous
default-recovery policy; unexpected storage failures remain failures. Settings
write only the chosen preference, and settlement handling reads the current
records. No second in-memory preference store, synchronization or alternative
storage decoder is installed.

The effect observes only the Host's root settlement projection. It stays quiet
while the document has focus, honors each channel's preference, requests OS
authorization through the public capability and preserves the captured Session.
Locale changes update copy without resubscribing to settled facts. Recorded error
text stays recorded text. The Host namespaces notification IDs by installed
component and returns the opaque Session target when the user opens a notification.
That routing remains available independently of the optional policy.

Withdrawal stops new observations and sends. Accepted authorization requests and
audio work remain joined by the captured component lifetime; a late permission
grant can update the Host's permission projection without starting another send.
Each chime closes its audio context on completion or cancellation, including when
the audio clock is suspended. Playback and release failures keep their causes.

Rebuild the public component SDK and graphical Host together, then select the
current package bytes. Notification preferences now appear under Extensions in
Settings. Existing preference records keep their values.
