import { expect, it } from "vitest";
import versions from "../../../versions.json";
import { supportedApiSchema, wellVersion } from "./versions";

it("uses the root Well version and supported API schema", () => {
  expect(wellVersion).toBe(versions.well.version);
  expect(supportedApiSchema).toBe(versions.well.support_api_schema);
});
