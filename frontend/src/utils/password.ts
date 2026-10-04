export function passwordError(password: string): string {
  if ([...password].length < 8 || new TextEncoder().encode(password).length > 1024) return '密码至少 8 位，且不超过 1024 字节'
  if (!/[A-Z]/.test(password) || !/[a-z]/.test(password) || !/[0-9]/.test(password)) return '密码必须包含大写字母、小写字母和数字'
  return ''
}
