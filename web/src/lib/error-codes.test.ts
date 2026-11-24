// @ts-nocheck -- runs with node:test; the project has no @types/node
import { test } from "node:test";
import assert from "node:assert/strict";
import { hasText } from "./errors.js";
import { specificErrorCodes } from "../api/types.gen.js";

test("every specific error code of the backend has a text in the web", () => {
  assert.deepEqual(specificErrorCodes.filter((code) => !hasText(code)), []);
});
