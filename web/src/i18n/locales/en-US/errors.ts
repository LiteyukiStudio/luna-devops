const errors = {
  auth: {
    unauthorized: 'Please sign in first',
    forbidden: 'You do not have permission to perform this action',
    login: { invalid: 'Email or password is incorrect' },
    session: {
      missing: 'Please sign in first',
      expired: 'Your session has expired. Please sign in again',
    },
    account: { disabled: 'This account is unavailable. Contact a platform administrator' },
  },
  application: {
    delete_in_progress: 'The application is being deleted. Wait for resource cleanup to finish, or retry after deletion fails.',
  },
  billing: {
    insufficient_balance: 'The billing owner balance is insufficient. Billing controls blocked this operation. Recharge or contact a platform administrator.',
    owner_required: 'This project space has no billing owner. Contact a platform administrator.',
    project_forbidden: 'You do not have access to this project space billing data',
    project_required: 'Select a project space',
    user_required: 'Select a user account',
    rate_rule_invalid_price: 'Billing rule price must be a non-negative number',
    rate_rule_meter_required: 'Billing rule meter is required',
    rate_rule_unknown: 'Unknown billing rule meter',
    transaction_invalid: 'The balance adjustment request is invalid',
    transaction_invalid_amount: 'Balance adjustment amount must be a non-zero number',
  },
  config: {
    admin: { required: 'Confirm that the current account has platform administrator permission.' },
  },
  request: {
    invalid: 'The request parameters are invalid',
    invalid_json: 'The request JSON is invalid',
    failed: 'The request failed. Try again later',
  },
  resource: {
    not_found: 'The resource does not exist or has been deleted',
    conflict: 'The resource state has changed. Refresh and try again',
  },
  network: {
    failed: 'Cannot connect to the platform backend. Check the local service, network proxy, or VPN settings.',
  },
  rate_limited: 'Too many requests. Try again later',
  internal_error: 'The service is temporarily unavailable. Try again later',
  git: {
    network_failed: 'Failed to connect to the Git platform. Check server network, proxy/VPN, DNS resolution, or FakeIP settings and try again.',
    token_refresh_failed: 'Git token refresh failed. Reauthorize or check the credential.',
    upstream_failed: 'Git upstream request failed. Try again later.',
    permission_denied: 'The Git credential does not have enough permission to access the repository or configure the webhook. Check its permissions and try again.',
    repository_not_found: 'The Git repository does not exist, or the current credential cannot access it. Check the repository and Git credential.',
    validation_failed: 'The Git platform rejected this configuration request. Check the repository, webhook callback URL, and credential permissions.',
    webhook_callback_unreachable: 'The webhook callback URL cannot be reached by GitHub/Gitea from the public Internet. Configure a public PUBLIC_BASE_URL and reconfigure the webhook.',
    webhook_callback_invalid: 'The webhook callback URL is invalid. Configure PUBLIC_BASE_URL with an http/https URL and reconfigure the webhook.',
    webhook_already_exists: 'A webhook with the same callback URL may already exist in this repository. Check the Git platform and retry or use the existing webhook.',
    webhook_rate_limited: 'The Git platform is temporarily limiting webhook creation requests. Try again later.',
  },
}

export default errors
