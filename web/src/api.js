// netcfg 管理台 HTTP 封装：统一处理 envelope、超时与友好错误提示。
// 后端返回稳定错误码，前端映射中文文案，不向用户透传底层技术错误。
const TIMEOUT_MS = 15000

// 错误码到中文文案的映射，未知错误码回退兜底文案
const ERROR_MESSAGES = {
  invalid_argument: '提交的内容有误，请检查后重试',
  not_found: '未找到对应的配置，请刷新后重试',
  unsupported: '当前系统不支持此操作',
  permission_denied: '没有执行此操作的权限，请检查权限后重试',
  command_failed: '操作未能完成，请稍后重试',
  timeout: '请求超时，请检查网络后重试',
  network: '无法连接后端服务，请确认服务已启动'
}

const FALLBACK_MESSAGE = '操作失败，请稍后重试'

function messageOf(code) {
  return ERROR_MESSAGES[code] || FALLBACK_MESSAGE
}

export class ApiError extends Error {
  constructor(message, code, detail) {
    super(message)
    this.name = 'ApiError'
    this.code = code || 'unknown'
    // 后端原始 message 仅用于排查，不在界面上展示
    this.detail = detail || ''
  }
}

async function request(path, { method = 'GET', body } = {}) {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), TIMEOUT_MS)
  let resp
  try {
    resp = await fetch(path, {
      method,
      headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
      body: body !== undefined ? JSON.stringify(body) : undefined,
      signal: controller.signal
    })
  } catch (err) {
    if (err && err.name === 'AbortError') {
      throw new ApiError(messageOf('timeout'), 'timeout')
    }
    throw new ApiError(messageOf('network'), 'network')
  } finally {
    clearTimeout(timer)
  }

  if (!resp.ok) {
    let code = ''
    let detail = ''
    try {
      const payload = await resp.json()
      if (payload && payload.error) {
        code = payload.error.code || ''
        detail = payload.error.message || ''
      }
    } catch (e) {
      // 响应体不是 JSON 时使用兜底文案
    }
    throw new ApiError(messageOf(code), code || `http_${resp.status}`, detail)
  }

  const text = await resp.text()
  if (!text) return null
  let payload = null
  try {
    payload = JSON.parse(text)
  } catch (e) {
    return null
  }
  if (payload && typeof payload === 'object' && Object.prototype.hasOwnProperty.call(payload, 'data')) {
    return payload.data
  }
  return payload
}

export function getVersion() {
  return request('/api/version')
}

export function getInterfaces() {
  return request('/api/interfaces')
}

export function getInterface(name) {
  return request(`/api/interfaces/${encodeURIComponent(name)}`)
}

export function setIP(name, config) {
  return request(`/api/interfaces/${encodeURIComponent(name)}/ip`, { method: 'POST', body: config })
}

export function setDNS(name, config) {
  return request(`/api/interfaces/${encodeURIComponent(name)}/dns`, { method: 'POST', body: config })
}

export function scanWifi() {
  return request('/api/wifi')
}

export function connectWifi({ interface: iface, ssid, password }) {
  return request('/api/wifi/connect', {
    method: 'POST',
    body: { interface: iface || '', ssid, password: password || '' }
  })
}

export function listHosts() {
  return request('/api/hosts')
}

export function addHost({ ip, hostname }) {
  return request('/api/hosts', { method: 'POST', body: { ip, hostname } })
}

export function removeHost(hostname) {
  return request(`/api/hosts/${encodeURIComponent(hostname)}`, { method: 'DELETE' })
}

export function getProxy() {
  return request('/api/proxy')
}

export function setProxy(config) {
  return request('/api/proxy', { method: 'PUT', body: config })
}
