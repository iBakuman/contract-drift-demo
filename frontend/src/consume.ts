/**
 * Consumes the generated client three times, once per server scenario.
 *
 * Everything in this file passes `tsc --strict` with no errors. That is the
 * point: the type checker has nothing to complain about, because the types it
 * was given are an accurate reading of the contract. The contract is not what
 * broke.
 */
import { listWidgets, type WidgetList } from './generated/widgets.js'

async function run(scenario: 'ok' | 'nil-slice' | 'omit-optional', what: string) {
  console.log(`\n── scenario: ${scenario} ─── ${what}`)

  const data: WidgetList = await listWidgets({ scenario })
  console.log(`   raw response: ${JSON.stringify(data)}`)

  renderList(data)
  renderTitles(data)
}

/**
 * Failure mode A lands here.
 *
 * `items` is typed `Widget[] | undefined`, so it needs a default before it can
 * be mapped over. A destructuring default is the idiomatic way to write that,
 * and TypeScript agrees: inside this function `items` has type `Widget[]`.
 *
 * A destructuring default only fires on `undefined`. `null` goes straight
 * through it, keeping the declared type and losing the runtime guarantee.
 */
function renderList({ items = [] }: WidgetList) {
  try {
    const ids = items.map((w) => w.id)
    console.log(`   list  → rendered ${ids.length} widget(s): ${ids.join(', ')}`)
  } catch (err) {
    console.log(`   list  → 💥 ${(err as Error).message}`)
  }
}

/**
 * Failure mode B lands here.
 *
 * `label` is optional in the contract. Every response seen during development
 * had one, so reading it directly looks safe, and the non-null assertion makes
 * the type checker agree.
 */
function renderTitles({ items = [] }: WidgetList) {
  if (!Array.isArray(items)) {
    console.log('   title → skipped; items was not an array (that is mode A, above)')
    return
  }

  try {
    const titles = items.map((w) => w.label!.toUpperCase())
    console.log(`   title → ${titles.join(', ')}`)
  } catch (err) {
    console.log(`   title → 💥 ${(err as Error).message}`)
  }

  const lines = items.map((w) => `${w.id}=${w.label}`)
  console.log(`   label → ${lines.join(', ')}`)
}

await run('ok', 'server builds the slice with make(...)')
await run('nil-slice', 'server sends a null the contract forbids')
await run('omit-optional', 'server legally leaves an optional field out')

console.log()
