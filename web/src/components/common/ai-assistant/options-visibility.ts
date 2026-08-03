export function shouldDisplayAIOptions(desktop: boolean, conversationsOpen: boolean) {
  return desktop || !conversationsOpen
}
