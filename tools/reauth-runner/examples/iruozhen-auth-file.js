// Paste this function body into Custom JavaScript; select OAuth authorization file.
// input: email----password----2FA secret (one account only).
const root = 'https://sms.iruozhen.com';
const text = typeof input === 'string' ? input.trim() : '';
const parts = text.split('----');
if (parts.length !== 3 || parts.some(part => !part.trim()) || /[\r\n]/.test(text)) {
  return { type: 'error', error: { code: 'INVALID_INPUT', message: '请输入单个账号：邮箱----密码----2FA密钥', retryable: false } };
}
const email = parts[0].trim().toLowerCase();
const cookies = new Map();
const deadline = Date.now() + 26000;
const fail = (code, message, retryable = false) => ({ type: 'error', error: { code, message, retryable } });
async function request(path, body) {
  const remaining = deadline - Date.now();
  if (remaining <= 0) throw new Error('deadline');
  const response = await fetch(root + path, {
    method: body === undefined ? 'GET' : 'POST',
    redirect: 'error',
    signal: AbortSignal.timeout(Math.min(remaining, 10000)),
    headers: {
      Accept: 'application/json',
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
      ...(cookies.size ? { Cookie: [...cookies].map(([name, value]) => name + '=' + value).join('; ') } : {})
    },
    ...(body === undefined ? {} : { body: JSON.stringify(body) })
  });
  const setCookies = response.headers.getSetCookie?.() || response.headers.raw?.()['set-cookie'] || [];
  for (const cookie of setCookies) {
    const pair = cookie.split(';', 1)[0];
    const equal = pair.indexOf('=');
    if (equal > 0) cookies.set(pair.slice(0, equal), pair.slice(equal + 1));
  }
  let data;
  try { data = await response.json(); } catch { throw new Error('invalid response'); }
  return { ok: response.ok, status: response.status, data };
}
try {
  const submitted = await request('/api/submit', { text, account_first: true });
  if (!submitted.ok || submitted.data.ok === false) {
    return fail('SUBMIT_REJECTED', '网站拒绝提交（HTTP ' + submitted.status + '）；若要求 CDK，请先在网站完成 CDK 授权。');
  }
  const created = submitted.data.created;
  if (!Array.isArray(created) || created.length !== 1 || !Number.isSafeInteger(created[0].task_id)) {
    return fail('TASK_ID_MISSING', '提交响应未返回唯一任务编号。');
  }
  const taskID = created[0].task_id;
  while (Date.now() < deadline) {
    const status = await request('/api/tasks/' + taskID);
    if (!status.ok) return fail('TASK_QUERY_FAILED', '查询任务失败（HTTP ' + status.status + '）。');
    const task = status.data.task;
    if (!task) return fail('INVALID_TASK_RESPONSE', '网站未返回任务状态。');
    if (task.status === 'failed' || task.status === 'cancelled') {
      return fail('REMOTE_AUTH_FAILED', '网站授权任务失败或已取消，请在网站查看任务详情。');
    }
    if (task.status === 'success') {
      const downloaded = await request('/api/download/' + taskID + '?format=sub2api');
      if (!downloaded.ok) return fail('DOWNLOAD_FAILED', '下载失败（HTTP ' + downloaded.status + '）。');
      const accounts = downloaded.data.accounts;
      if (!Array.isArray(accounts) || accounts.length !== 1) return fail('INVALID_AUTH_FILE', '下载文件必须包含一个账号。');
      const account = accounts[0];
      const auth = account.credentials;
      if (account.platform !== 'openai' || account.type !== 'oauth' || !auth?.access_token || !auth.refresh_token) {
        return fail('INVALID_AUTH_FILE', '下载文件缺少 OpenAI OAuth 凭据。');
      }
      if (typeof auth.email !== 'string' || auth.email.toLowerCase() !== email) {
        return fail('IDENTITY_MISMATCH', '下载文件的邮箱与输入账号不一致。');
      }
      return { type: 'auth_file', auth };
    }
    await new Promise(resolve => setTimeout(resolve, Math.min(1500, Math.max(0, deadline - Date.now()))));
  }
  return fail('REMOTE_TASK_PENDING', '网站任务仍在处理中；当前自定义代码限时 30 秒，请任务完成后再试。', true);
} catch {
  return fail('REMOTE_REQUEST_FAILED', '网站请求失败或超时；请检查网络、代理及网站任务状态。', true);
}
