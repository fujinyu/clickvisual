export default {
  dev: {
    "/api/": {
      target: "http://127.0.0.1:9001",
      changeOrigin: true,
      pathRewrite: { "^": "" },
    },
    "/v2": {
      target: "http://127.0.0.1:9001",
      changeOrigin: true,
    },
  },
  test: {},
  pre: {},
};
