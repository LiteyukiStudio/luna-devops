import auth from './ko-KR/auth'
import bootstrap from './ko-KR/bootstrap'
import common from './ko-KR/common'
import errors from './ko-KR/errors'
import languages from './ko-KR/languages'
import loginPage from './ko-KR/loginPage'
import oauthApps from './ko-KR/oauthApps'
import pagination from './ko-KR/pagination'
import root from './ko-KR/root'
import theme from './ko-KR/theme'
import time from './ko-KR/time'

const koKR = {
  ...root,
  languages,
  common,
  time,
  errors,
  auth,
  pagination,
  oauthApps,
  theme,
  loginPage,
  bootstrap,
}

export default koKR
