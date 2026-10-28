export class MacComputerUseClient {
  async request() {
    globalThis.nativeFixture.events.push("native-end");
  }
}
