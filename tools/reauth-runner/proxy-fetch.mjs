import fetch from 'node-fetch'
import { HttpProxyAgent } from 'http-proxy-agent'
import { HttpsProxyAgent } from 'https-proxy-agent'
import { SocksProxyAgent } from 'socks-proxy-agent'

// Each task keeps its selected route for login, scripts and token exchange.
export function createProxyFetch(proxyURL) {
  let agent
  let agents = []
  if (proxyURL) {
    const proxy = new URL(proxyURL)
    if (['socks:', 'socks4:', 'socks5:', 'socks4a:', 'socks5h:'].includes(proxy.protocol)) {
      agent = new SocksProxyAgent(proxyURL)
      agents = [agent]
    } else if (['http:', 'https:'].includes(proxy.protocol)) {
      const http = new HttpProxyAgent(proxyURL), https = new HttpsProxyAgent(proxyURL)
      agents = [http, https]
      agent = target => target.protocol === 'https:' ? https : http
    } else throw new Error('Unsupported authorization proxy protocol')
  }
  return {
    fetch: (url, options = {}) => fetch(url, { ...options, agent }),
    close: () => agents.forEach(item => item.destroy())
  }
}
