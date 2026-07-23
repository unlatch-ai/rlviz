import { describe, expect, it } from "vitest";
import { defaultWorkspaceModuleSizes } from "./workspaceDock";

describe("default workspace module sizes", () => {
  it("leaves most of a desktop viewport to rollout and detail modules", () => {
    expect(defaultWorkspaceModuleSizes(1440, 900)).toEqual({
      collectionWidth: 259,
      guideWidth: 446,
      settingsHeight: 180,
    });
  });

  it("keeps the modules usable in a compact viewport", () => {
    expect(defaultWorkspaceModuleSizes(1024, 700)).toEqual({
      collectionWidth: 220,
      guideWidth: 320,
      settingsHeight: 150,
    });
  });
});
