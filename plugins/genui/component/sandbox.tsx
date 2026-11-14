import { useLayoutEffect, useRef } from "react";

function ProgramFrame({
  title,
  html,
  height,
}: {
  title: string;
  html: string;
  height: number;
}) {
  const element = useRef<HTMLIFrameElement>(null);
  useLayoutEffect(() => {
    const frame = element.current;
    if (!frame) return;
    let port: MessagePort | undefined;
    const connect = () => {
      port?.close();
      const channel = new MessageChannel();
      const id = crypto.randomUUID();
      port = channel.port1;
      channel.port1.onmessage = (event) => {
        if (
          port !== channel.port1 ||
          event.data?.type !== "flame-genui-ready" ||
          event.data.id !== id
        )
          return;
        channel.port1.onmessage = null;
        channel.port1.postMessage({
          type: "flame-genui-content",
          id,
          title: frame.title,
          html,
        });
      };
      frame.contentWindow?.postMessage(
        { type: "flame-genui-connect", id },
        "*",
        [channel.port2],
      );
    };
    frame.addEventListener("load", connect);
    return () => {
      frame.removeEventListener("load", connect);
      port?.close();
      port = undefined;
    };
  }, [html]);
  return (
    <iframe
      ref={element}
      title={title}
      src="/genui-sandbox.html"
      sandbox="allow-scripts"
      referrerPolicy="no-referrer"
      allow="camera 'none'; microphone 'none'; geolocation 'none'; clipboard-read 'none'; clipboard-write 'none'"
      loading="lazy"
      style={{ width: "100%", height, border: 0 }}
    />
  );
}

export function SandboxView({
  title,
  html,
  height = 360,
}: {
  title: string;
  html: string;
  height?: number;
}) {
  return <ProgramFrame key={html} title={title} html={html} height={height} />;
}
