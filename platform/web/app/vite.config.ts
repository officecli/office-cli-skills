import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const mockApi = () => {
  return {
    name: 'mock-api',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        if (req.url?.startsWith('/api/')) {
          res.setHeader('Content-Type', 'application/json')
          
          if (req.url === '/api/auth/me') {
            res.end(JSON.stringify({ data: { id: 1, email: 'mock@example.com', name: 'Mock User', status: 'active' } }))
            return
          }
          if (req.url === '/api/app/overview') {
            res.end(JSON.stringify({ data: {
              api_key_count: 2,
              total_remaining: 1000,
              reward_remaining: 500,
              invite_code: 'MOCK123',
              invite_limit: 10,
              invite_remaining: 5,
              reward_per_invite: 100,
              referral_count: 5,
              activated_referral_count: 2,
              discord_connected: true,
              discord_guild_member: true,
              recent_usage_count: 10,
              recent_orders_count: 2,
              pricing: []
            } }))
            return
          }
          if (req.url === '/api/app/growth') {
            res.end(JSON.stringify({ data: {
              invite_code: 'MOCK123',
              invite_limit: 10,
              invite_remaining: 5,
              reward_per_invite: 100,
              reward_remaining: 500,
              reward_grants: [],
              referrals: []
            } }))
            return
          }
          if (req.url === '/api/app/api-keys') {
            res.end(JSON.stringify({ data: [
              { id: 1, key_prefix: 'sk-mock1...', status: 'active', plan_name: 'Free', quota_used: 100, quota_remaining: 900, created_at: new Date().toISOString() },
              { id: 2, key_prefix: 'sk-mock2...', status: 'disabled', plan_name: 'Pro', quota_used: 5000, quota_remaining: 0, created_at: new Date().toISOString() }
            ] }))
            return
          }
          if (req.url === '/api/app/orders') {
            res.end(JSON.stringify({ data: [
              { id: 1, status: 'completed', currency: 'USD', amount_total: 1000, pack_code: 'basic', pack_name: 'Basic Pack', quota_amount: 10000, created_at: new Date().toISOString() }
            ] }))
            return
          }
          if (req.url === '/api/app/usage-events') {
            res.end(JSON.stringify({ data: [
              { id: 1, mode: 'chat', action: 'completion', result: 'success', created_at: new Date().toISOString(), fingerprint_hash: 'abc', prompt_tokens: 10, completion_tokens: 20 }
            ] }))
            return
          }
          if (req.url === '/api/pricing') {
            res.end(JSON.stringify({ data: [
              { code: 'basic', name: 'Basic', description: 'Basic Pack', currency: 'USD', amount_total: 1000, quota_amount: 10000 },
              { code: 'pro', name: 'Pro', description: 'Pro Pack', currency: 'USD', amount_total: 5000, quota_amount: 100000 }
            ] }))
            return
          }
          
          res.end(JSON.stringify({ data: {} }))
          return
        }
        next()
      })
    }
  }
}

export default defineConfig({
  base: '/app/',
  plugins: [react(), tailwindcss(), mockApi()],
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
  },
})
