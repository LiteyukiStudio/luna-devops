const bootstrap = {
  title: '초기화',
  description: '첫 번째 플랫폼 관리자 계정을 생성합니다.',
  email: '관리자 이메일',
  emailHint: '첫 번째 플랫폼 관리자 계정의 로그인 이메일이며, 이후 OIDC 바인딩 시 로컬 계정을 찾는 기준이 됩니다.',
  name: '관리자 이름',
  nameRequired: '관리자 이름을 입력하세요',
  nameHint: '사이드바와 감사 로그에 표시되는 관리자 이름입니다.',
  passwordHint: '로컬 관리자 비밀번호(8자 이상). 프로덕션 환경에서는 강력한 비밀번호를 사용하고 빠르게 OIDC를 설정하세요.',
  token: '초기화 토큰',
  tokenHint: '플랫폼 배포 관리자가 서버 측 설정으로 제공합니다. 최초 초기화에만 사용됩니다.',
  tokenRequired: '초기화 토큰을 입력하세요',
  create: '관리자 생성',
  success: '플랫폼 관리자가 초기화되었습니다',
  note: '플랫폼에 PlatformAdmin이 없을 때만 초기화가 허용됩니다. 프로덕션 환경에는 개발용 기본 계정이 표시되지 않습니다.',
}

export default bootstrap
