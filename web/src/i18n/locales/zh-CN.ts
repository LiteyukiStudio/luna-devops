import languages from './languages'
import auth from './zh-CN/auth'
import common from './zh-CN/common'
import errors from './zh-CN/errors'
import loginPage from './zh-CN/loginPage'
import oauthApps from './zh-CN/oauthApps'
import pagination from './zh-CN/pagination'
import root from './zh-CN/root'
import theme from './zh-CN/theme'
import time from './zh-CN/time'

const zhCN = {
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
}

export default zhCN
