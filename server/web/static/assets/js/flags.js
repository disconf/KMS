/* ============================================================================
   flags.js — 功能开关。

   开关只控制「入口是否出现」，不控制路由：路由删掉会让收藏与外链直接 404，
   而入口收起是随时可逆的产品决定。所有开关默认值集中在这里，
   避免同一件事在 app.js 和某个页面里各写一个常量、慢慢走岔。
   ========================================================================= */

export const FLAGS = {
  /** 悬赏 / 订单 / 接单大厅入口。 */
  service: false,

  /**
   * 专家广场入口。
   *
   * 收起后：顶部导航不再出现「专家广场」，`#/experts` 列表页仍可直连
   * （页面会显示"内容整理中"提示），单个专家主页 `#/experts/:id` 完全保留——
   * 订单页、问答回答的专家角标都还在往那里跳转。
   */
  expertPlaza: false,
};

/** 徽章门槛，必须与 service.BadgeOf 的阈值一致。 */
export const BADGE_LEVELS = [
  { at: 0, label: '新手' },
  { at: 10, label: '活跃' },
  { at: 50, label: '贡献者' },
  { at: 200, label: '资深贡献者' },
];

/** 按积分算徽章（后端也会下发，这里用于本地即时展示）。 */
export function badgeOf(points) {
  const n = Number(points) || 0;
  let label = BADGE_LEVELS[0].label;
  BADGE_LEVELS.forEach((b) => { if (n >= b.at) label = b.label; });
  return label;
}
