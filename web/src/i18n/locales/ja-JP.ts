import auth from './ja-JP/auth'
import bootstrap from './ja-JP/bootstrap'
import common from './ja-JP/common'
import errors from './ja-JP/errors'
import languages from './ja-JP/languages'
import loginPage from './ja-JP/loginPage'
import oauthApps from './ja-JP/oauthApps'
import pagination from './ja-JP/pagination'
import root from './ja-JP/root'
import theme from './ja-JP/theme'
import time from './ja-JP/time'

const jaJP = {
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

export default jaJP
