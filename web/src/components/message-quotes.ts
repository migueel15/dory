import { readCodeRef, type CodeRefData } from "./code-ref";

type MessagePart = { kind: "text" | "quote"; text: string } | { kind: "code"; text: string; ref: CodeRefData };

export function splitMessageQuotes(message: string): MessagePart[] {
  const lines = message.split("\n");
  const parts: MessagePart[] = [];
  let textStart = 0;
  let fence: string | undefined;
  const appendText = (end: number) => {
    let text = lines.slice(textStart, end).join("\n").trim();
    if (parts.at(-1)?.kind === "quote") {
      text = text.replace(/^\*\*Respuesta a la cita\*\*\n\n/, "");
    }
    if (text) parts.push({ kind: "text", text });
  };

  for (let index = 0; index < lines.length; index++) {
    const marker = /^ {0,3}(`{3,}|~{3,})(.*)$/.exec(lines[index]);
    if (marker) {
      if (!fence) fence = marker[1];
      else if (marker[1].startsWith(fence) && !marker[2].trim()) fence = undefined;
      continue;
    }
    if (!fence) {
      const found = readCodeRef(lines, index);
      if (found) {
        appendText(index);
        parts.push({ kind: "code", text: lines.slice(index, found.last + 1).join("\n"), ref: found.ref });
        index = found.last;
        textStart = index + 1;
        continue;
      }
    }
    if (fence || !/^> «[^»]+»$/.test(lines[index]) || !lines[index + 1]?.startsWith(">")) continue;

    appendText(index);
    const quoted: string[] = [];
    while (lines[index + 1]?.startsWith(">")) {
      quoted.push(lines[++index].replace(/^> ?/, ""));
    }
    parts.push({ kind: "quote", text: quoted.join("\n") });
    textStart = index + 1;
  }
  if (!textStart) return [{ kind: "text", text: message }];
  appendText(lines.length);
  return parts;
}
