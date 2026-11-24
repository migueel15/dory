// @ts-nocheck -- runs with node:test; the project has no @types/node
import { test } from "node:test";
import assert from "node:assert/strict";
import { errorText, textFor } from "./errors.js";

test("a known code gives its Spanish text", () => {
  assert.equal(textFor("thread_busy", "wait"), "espera a que termine el turno antes de cambiar de proveedor");
  assert.equal(textFor("dirty_worktree", "x"), "el worktree tiene cambios sin commitear");
});

test("a reason refines a generic code and params fill the text", () => {
  assert.equal(textFor("invalid_argument", "x", { reason: "pid_invalid" }), "PID inválido");
  assert.equal(textFor("invalid_account", "x", { reason: "multiple_accounts_unsupported", driver: "opencode" }), "opencode no admite varias cuentas");
  assert.equal(textFor("node_offline", "x", { thread: true }), "el nodo que ejecuta este hilo no está conectado");
  assert.equal(textFor("node_offline", "x"), "el nodo no está conectado");
});

test("an unknown code or reason falls back to the message", () => {
  assert.equal(textFor("not_found", "thread not found"), "thread not found");
  assert.equal(textFor("invalid_argument", "title is required", { reason: "nope" }), "title is required");
  assert.equal(textFor(undefined, "Bad Gateway"), "Bad Gateway");
});

test("errorText reads the code of an API error and the message of any other", () => {
  const api = Object.assign(new Error("ignored"), { code: "update_refused", detail: "x", params: { reason: "busy" } });
  assert.equal(errorText(api), "hay turnos en marcha en este nodo");
  assert.equal(errorText(new Error("plain")), "plain");
  assert.equal(errorText("not an error"), "");
});
