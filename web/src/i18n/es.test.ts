// @ts-nocheck -- runs with node:test; the project has no @types/node
import { test } from "node:test";
import assert from "node:assert/strict";
import { messageText } from "./es.js";

test("a code is translated with its params", () => {
  assert.equal(messageText({ code: "context.cleared", text: "" }), "Contexto limpiado");
  assert.equal(messageText({ code: "model.change_failed", params: { error: "x" }, text: "" }), "No se pudo cambiar el modelo: x");
});

test("a message without a known code shows its own text", () => {
  assert.equal(messageText({ text: "Contexto limpiado" }), "Contexto limpiado");
  assert.equal(messageText({ code: "nope", text: "hola" }), "hola");
});
