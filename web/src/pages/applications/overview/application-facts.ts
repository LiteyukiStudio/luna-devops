export function latestByDate<T>(items: T[], dateOf: (item: T) => string | undefined) {
  return items.reduce<T | undefined>((latest, item) => {
    if (!latest)
      return item
    return new Date(dateOf(item) ?? '').getTime() > new Date(dateOf(latest) ?? '').getTime() ? item : latest
  }, undefined)
}
