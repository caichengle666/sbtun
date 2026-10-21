import { defineConfig } from 'vite'

// 安卓上有的系统自带 WebView 内核很旧（不认 inset 等新写法），CSS/JS 按旧版 Chrome 降级输出。
export default defineConfig({
  build: { target: ['es2019', 'chrome70'], cssTarget: 'chrome70' },
})
