# 生成式 API 契约的两种失效方式

[English](README.md)

一个能跑起来的五分钟 Demo，演示契约先行（contract-first）技术栈里一个普遍问题：一份 OpenAPI 文档、一个生成的服务端、一个生成的客户端，以及一个两边生成器都看不见的 bug。

它刻意做得极小 —— 一个端点、一个响应对象、一个可选数组。所有代码都能编译，类型检查一句话都不说，客户端照样崩。

有意思的地方在于：这里有**两种不同的失效**，远看长得一样，但需要**两道不同的防线**。把它们混为一谈，是团队最后只装了一道防线却以为两边都保住了的原因。

## 跑起来

需要 Go 1.24+、Node 20+、pnpm。生成的代码已经 check in，所以不装生成器也能跑。

```bash
cd frontend && pnpm install && cd ..

make demo       # 起服务端，打三次，看什么坏了
make check      # 服务端那道防线：校验真实发出去的字节
make typecheck  # 客户端的视角：tsc --strict 一句话都没有
```

`make demo` 的输出：

```
── scenario: ok ─── server builds the slice with make(...)
   raw response: {"generatedAt":"…","items":[{"id":"w-1","label":"left hinge"},…]}
   list  → rendered 2 widget(s): w-1, w-2
   title → LEFT HINGE, RIGHT HINGE

── scenario: nil-slice ─── server sends a null the contract forbids
   raw response: {"generatedAt":"…","items":null}
   list  → 💥 Cannot read properties of null (reading 'map')

── scenario: omit-optional ─── server legally leaves an optional field out
   raw response: {"generatedAt":"…","items":[{"id":"w-1"},{"id":"w-2"}]}
   list  → rendered 2 widget(s): w-1, w-2
   title → 💥 Cannot read properties of undefined (reading 'toUpperCase')
   label → w-1=undefined, w-2=undefined
```

## 契约本身

全部内容都在 [`openapi.yaml`](openapi.yaml) 里。关键是两个字段：

```yaml
WidgetList:
  properties:
    items:          # 可选、是数组、并且没有声明 nullable
      type: array
      items: { $ref: '#/components/schemas/Widget' }

Widget:
  required: [id]
  properties:
    label:          # 可选 —— 服务端可以不给
      type: string
```

这两行要慢慢读，因为两种失效都藏在「可选」这个词的两种不同含义里。

对 `items` 来说，可选的意思是：**这个 key 可以不出现，或者它的值是一个数组。** `null` 不在这个清单上。契约从来没说这个字段可以是 null，所以客户端有理由认定它永远不是。

对 `label` 来说，可选是同一个意思 —— 而且服务端真的用了这个自由。这是允许的。得由客户端来兜。

---

## 失效模式 A —— 服务端发出了契约不允许的值

### 发生了什么

`oapi-codegen` 读到 `items: 可选数组`，生成出来是：

```go
type WidgetList struct {
    GeneratedAt string    `json:"generatedAt"`
    Items       *[]Widget `json:"items,omitempty"`
}
```

可选变成了指针。这是 Go 的编码器为「这个 key 可能不在」提供的唯一工具，而 `omitempty` 就是让这个 key 被丢掉的办法。

现在看 [`main.go`](backend/main.go) 里填这个字段的两条路。两条都是再普通不过的 Go，两条都过得了 review。

```go
// wellBehaved
items := make([]api.Widget, 0, len(rows))
for _, row := range rows { items = append(items, …) }
return api.WidgetList{Items: &items}          // 空结果 → []

// nilSlice
items := byGroup["archived"]                  // 没有这个 key → nil 切片
return api.WidgetList{Items: &items}          // 空结果 → null
```

第二条发出去的就是 `"items": null`。

机制在这里，值得慢慢说，因为这就是全部要点：**`omitempty` 问的是指针，不是切片。** 指针是实的 —— 它指向一个确实存在的变量 —— 所以 key 留下了。接着编码器去问切片「你变成 JSON 长什么样」，而 nil 切片的答案是 `null`。

`omitempty` 穿不透那层指针，而 Go 里每一个 `[]T` 都能是 nil。所以**这个 API 里每一个数组字段都有同样的能力**，不管生成器怎么配。没有任何代码生成选项能拿掉它。

### 为什么客户端预料不到

`orval` 读的是同一份契约的同一行，生成出来是：

```ts
export interface WidgetList {
  generatedAt: string
  items?: Widget[]      // 要么不存在，要么是数组。永远不会是 null。
}
```

这是一个*正确*的解读。契约确实就是这么写的。于是客户端写下了处理可选数组的标准写法：

```ts
function renderList({ items = [] }: WidgetList) {
  const ids = items.map((w) => w.id)     // items: Widget[]  ← tsc 很确信
}
```

而解构默认值**只在 `undefined` 时生效**。`null` 直接穿过去，保留着 `Widget[]` 这个声明类型，然后走到了 `.map()`。

### 这件事的形状

没有人错。Go 类型是契约的忠实生成，TS 类型是同一份契约的忠实生成，客户端那段代码是处理可选数组的推荐写法。唯一断掉的那环是：服务端**发出了自己契约不允许的值**，而系统里没有任何东西在检查这件事。

这不是靠 code review 能解决的问题。Demo 把两种构造放在同一个文件里就是为了说明这点：`wellBehaved` 从来没出过这个 bug，`nilSlice` 出了，而区别是几行之外的一个 `make` 调用。

---

## 失效模式 B —— 服务端完全正确，是客户端假定得太多

### 发生了什么

`omitOptional` 没给 `label`。这完全在契约范围内 —— `label` 是可选的，服务端选择不填。

但客户端是照着开发期见过的那些响应写的，那时候 `label` 每次都在：

```ts
const titles = items.map((w) => w.label!.toUpperCase())
//                                     ↑ 非空断言
```

`tsc --strict` 本来会拦下 `w.label.toUpperCase()`。那个 `!` 就是让它闭嘴的办法，而闭嘴的理由听起来还挺正当：「服务端每次都会给的。」契约可从来没这么承诺过。

同一个错误还有更安静的版本，它连崩都不崩：

```ts
`${w.id}=${w.label}`     // → "w-1=undefined"，就这么渲染给用户看了
```

### 这件事的形状

两边都在契约之内。服务端行使了契约给它的一项自由，而客户端已经不再把这项自由当真了。这里根本没有「违规」可供检测 —— 这正是为什么 A 的那道防线在这儿一点用都没有。

---

## 并排看

|  | **A —— 非法的 null** | **B —— 没处理的可选** |
|---|---|---|
| 谁违了约 | 服务端 | 没有人 |
| 响应长什么样 | `"items": null` | `"label"` 这个 key 不存在 |
| 响应合法吗？ | **不合法** | **合法** |
| 客户端拿到什么 | 类型说是 `Widget[]` 的地方拿到 `null` | 代码假定是 `string` 的地方拿到 `undefined` |
| 校验响应能查出来吗？ | **能** | **不能** —— 压根没有可查的东西 |
| 防线该放在哪 | **服务端** | **客户端** |
| 防线是什么 | 在测试里拿 spec 校验真实发出的字节 | 把每个 `?` 当真：不写 `!`、不写 cast、在边界处兜底 |

---

## A 的防线：检查你真正发出去的字节

类型描述的是意图。防线必须去看产物。

[`backend/contractcheck`](backend/contractcheck/contractcheck.go) 是一个 `http.RoundTripper`，它拿生成类型所用的同一份 OpenAPI 文档去校验每一个响应 —— 生成器已经把 spec 嵌进去了，所以不存在第二份副本要维护。把它塞到集成测试本来就在用的 `http.Client` 底下，那些测试已经在做的每一次调用就都被检查了，**不用重写任何一个测试，不用改任何一条断言**：

```go
checker, _ := contractcheck.New(doc, nil)
client := &http.Client{Transport: checker}
```

它是**报告而不是中断**：契约违规是关于这个响应的一个发现，不是让请求失败的理由。测试继续跑自己的断言，失败输出里两者都在。

`make check` 拿三个场景跑它：

```
scenario ok             → 无违规
scenario nil-slice      → response body doesn't match schema #/components/schemas/WidgetList:
                          Error at "/items": Value is not nullable
scenario omit-optional  → 无违规
```

第三行不是实现上的缺漏，它就是正确答案。模式 B 产生的是一个合法响应。

### 这道防线做不到什么

它只能看见你的测试**实际打到过**的响应。没有集成测试覆盖的接口，或者有覆盖但从没走到空列表那条分支的，它一无所知。

但这仍然是一个真实的改变：它把「这东西能不能溜进生产」从*取决于所有人永远记得那个 `make` 调用*，变成了*取决于测试有没有打到那条路径* —— 后者是可以查、也可以补的。它不等于免疫。

## B 的防线：把可选当可选

服务端在这里帮不上任何忙，因为服务端什么都没做错。防线是客户端的纪律：

- **把逃生口封掉。** 开 `@typescript-eslint/no-non-null-assertion`，再加一条禁止对响应数据做 `as` 断言的规则。碰响应的代码里每一个 `!`，都是有人凭记忆推翻了契约的地方。
- **在边界处兜底，而不是在使用处。** 响应一进来就归一化一次，而不是在读它的那十二个地方各写一遍。
- **照契约给的自由写测试，而不是照服务端的习惯。** 把那个「可选字段缺失」的 fixture 写出来。服务端明天就可能开始这么干，而且不会通知你。

## 一个值得点名的陷阱

你可能注意到了，`items ?? []` 其实能扛过模式 A，因为 `??` 连 `null` 一起接住了。

确实能扛 —— 但是**碰巧扛住的**。作者写 `??` 是为了处理「字段不存在」，结果顺手也处理了一个契约声称永不会出现的值。解构默认值 `{ items = [] }` 表达的是同一个意图，却扛不住；先取 `.length` 的 `if (items) …` 也扛不住；任何相信了那个类型的代码都扛不住。

客户端的 null 合并不是对模式 A 的防线，它是一次抛硬币，赌作者当时随手选了哪种写法。对付「服务端发出非法值」的防线，是让服务端别发出来。

## 文件导览

```
scripts/demo.sh                         起服务端、跑客户端、收尾清理
openapi.yaml                            全部契约，约 40 行
backend/main.go                         三个 handler：正确的、模式 A、模式 B
backend/api/types.gen.go                oapi-codegen 产物（已 check in）
backend/contractcheck/contractcheck.go  响应校验器
backend/contract_test.go                证明它抓得住 A、抓不住 B
frontend/src/generated/widgets.ts       orval 产物（已 check in）
frontend/src/consume.ts                 客户端代码；过 tsc --strict
```

由 `oapi-codegen v2.8.0` 和 `orval v7.21.0` 生成。改了契约的话，`make generate` 会重跑两边。

---

Demo 里这两种失效都是对真实系统中进过生产的问题的重建。有意思的是机制本身，不是事故。
