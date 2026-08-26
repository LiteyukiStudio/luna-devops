import { defineConfig } from "vitest/config"

export default defineConfig({
  test: {
    projects: [
      {
        test: {
          name: "unit",
          include: ["tests/**/*.test.ts"],
          exclude: [
            "tests/context-compiler.test.ts",
            "tests/platform-catalog-retrieval.test.ts",
          ],
        },
      },
      {
        test: {
          name: "platform-catalog-contract",
          include: ["tests/platform-catalog-retrieval.test.ts"],
          globalSetup: ["tests/support/platform-catalog-global-setup.ts"],
        },
      },
      {
        test: {
          name: "cpu-intensive",
          include: ["tests/context-compiler.test.ts"],
          // 该文件包含大文本压缩用例，与其他文件并行时容易因 CPU 竞争超过默认 5s 超时
          fileParallelism: false,
          testTimeout: 30_000,
        },
      },
    ],
  },
})
