import { describe, expect, it } from "vitest";
import {
  assertLocaleResourceParity,
  createCliI18n,
  detectLocale,
  normalizeLocale,
} from "../../src/i18n/index.js";

describe("CLI locale selection", () => {
  it("normalizes supported locale variants", () => {
    expect(normalizeLocale("zh_Hans.UTF-8")).toBe("zh-CN");
    expect(normalizeLocale("en_GB.UTF-8")).toBe("en-US");
    expect(normalizeLocale("fr-FR")).toBeUndefined();
  });

  it("uses the documented locale priority and English fallback", () => {
    expect(detectLocale({
      explicit: "en-US",
      configured: "zh-CN",
      env: { LC_ALL: "zh_CN.UTF-8" },
    })).toBe("en-US");
    expect(detectLocale({
      configured: "zh-CN",
      env: { LC_ALL: "en_US.UTF-8" },
    })).toBe("zh-CN");
    expect(detectLocale({
      configured: "en-US",
      env: { LUNA_LANG: "zh_CN.UTF-8" },
    })).toBe("zh-CN");
    expect(detectLocale({
      explicit: "fr-FR",
      configured: "zh-CN",
      env: {},
    })).toBe("en-US");
    expect(detectLocale({
      env: { LC_ALL: "C", LANG: "zh_CN.UTF-8" },
    })).toBe("zh-CN");
    expect(detectLocale({ env: {}, runtimeLocale: "fr-FR" })).toBe("en-US");
  });

  it("keeps locale resources in parity and initializes i18next", async () => {
    expect(() => assertLocaleResourceParity()).not.toThrow();
    const i18n = await createCliI18n({ explicit: "zh-CN", env: {} });
    expect(i18n.t("common.empty")).toBe("没有找到数据。");
  });
});
