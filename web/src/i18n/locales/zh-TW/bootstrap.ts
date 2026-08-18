const bootstrap = {
  title: '初始化',
  description: '建立第一個平臺管理員賬號。',
  email: '管理員郵箱',
  emailHint: '第一個平臺管理員賬號的登入郵箱，也是後續繫結 OIDC 時查詢本地賬號的依據。',
  name: '管理員名稱',
  nameRequired: '請輸入管理員名稱',
  nameHint: '管理員在側邊欄和審計記錄裡顯示的名稱。',
  passwordHint: '本地管理員密碼，至少 8 位。生產環境請使用強密碼，並儘快配置 OIDC。',
  token: '初始化令牌',
  tokenHint: '由平臺部署管理員透過服務端配置提供，只用於首次初始化。',
  tokenRequired: '請輸入初始化令牌',
  create: '建立管理員',
  success: '平臺管理員已初始化',
  note: '僅當平臺沒有任何 PlatformAdmin 時允許初始化。生產環境不會顯示開發預設賬號。',
}

export default bootstrap
