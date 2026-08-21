import auth from './zh-TW/auth'
import bootstrap from './zh-TW/bootstrap'
import common from './zh-TW/common'
import errors from './zh-TW/errors'
import languages from './zh-TW/languages'
import loginPage from './zh-TW/loginPage'
import oauthApps from './zh-TW/oauthApps'
import pagination from './zh-TW/pagination'
import root from './zh-TW/root'
import theme from './zh-TW/theme'
import time from './zh-TW/time'

const zhTW = {
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

export default zhTW
