// 后端每个列表接口都有单页上限（药品、计划、库存流水等为 100，事件为 200），
// 前端传超过上限的值会被服务端静默截断成上限值——表现在界面上就是「有些条目凭空
// 消失了」。需要整本数据的地方（下拉选择、汇总列表）统一用这里逐页取全，
// 既不会被静默截断，也不会一次要回过多数据。

export interface PagedResult<T> {
  items: T[]
  total: number
}

export async function fetchAllPages<T>(
  fetchPage: (page: number, pageSize: number) => Promise<PagedResult<T>>,
  options: { pageSize?: number; maxPages?: number } = {},
): Promise<T[]> {
  const pageSize = options.pageSize ?? 100
  const maxPages = options.maxPages ?? 20
  const all: T[] = []
  for (let page = 1; page <= maxPages; page++) {
    const { items, total } = await fetchPage(page, pageSize)
    if (!items || items.length === 0) break
    all.push(...items)
    if (all.length >= total) break
  }
  return all
}
