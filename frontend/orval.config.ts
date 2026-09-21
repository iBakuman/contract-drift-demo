import { defineConfig } from 'orval'

export default defineConfig({
  widgets: {
    input: '../openapi.yaml',
    output: {
      target: './src/generated/widgets.ts',
      mode: 'single',
      client: 'fetch',
      baseUrl: 'http://localhost:8099',
      override: { fetch: { includeHttpResponseReturnType: false } },
    },
  },
})
