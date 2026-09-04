import auth from './en-US/auth'
import common from './en-US/common'
import errors from './en-US/errors'
import loginPage from './en-US/loginPage'
import oauthApps from './en-US/oauthApps'
import pagination from './en-US/pagination'
import root from './en-US/root'
import theme from './en-US/theme'
import time from './en-US/time'
import languages from './languages'

const enUS = {
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

export default enUS
