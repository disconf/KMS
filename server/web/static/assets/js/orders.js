/* ============================================================================
   orders.js — 订单域的公共知识：状态文案、金额格式、倒计时、可见动作。

   **状态文案只在这里出现一次。** 后端返回的 status 永远是机器值
   （pending_pay 等），中文标签一律由前端映射——原因见 internal/dto/order.go
   的注释：两处各写一份文案，早晚会对不上，而订单状态的文案错了是会被用户
   当成"订单卡住了"来投诉的。

   ORDER_TRANSITIONS 是 internal/model/order.go 的 orderTransitions 的镜像，
   只用于回答"这一单接下来可能变成什么"，**不作为校验依据**——真正的校验
   在服务端，前端抄一份只是为了把按钮显示对。
   ========================================================================= */

import { esc } from './util.js';

export const ORDER_STATUS = {
  pending_accept:  { label: '待接单',     tone: 'warn',   desc: '悬赏已发布，等待专家接单' },
  pending_pay:     { label: '待支付',     tone: 'warn',   desc: '专家已接单，请在截止前完成支付' },
  in_service:      { label: '服务中',     tone: 'info',   desc: '专家正在处理，可随时进入会话' },
  pending_confirm: { label: '待确认',     tone: 'info',   desc: '专家已标记完成，确认后才会结算' },
  completed:       { label: '已完成',     tone: 'ok',     desc: '服务已确认完成，等待结算入账' },
  settled:         { label: '已结算',     tone: 'ok',     desc: '款项已按分账结果结算给专家' },
  disputed:        { label: '争议处理中', tone: 'danger', desc: '平台已介入，处理期间款项冻结' },
  refunded:        { label: '已退款',     tone: 'danger', desc: '争议判定退款，款项已退回' },
  cancelled:       { label: '已取消',     tone: 'muted',  desc: '订单已关闭，不再继续' },
};

/** 与后端 orderTransitions 逐字对齐。改这里必须同时改 internal/model/order.go。 */
export const ORDER_TRANSITIONS = {
  pending_accept:  ['pending_pay', 'cancelled'],
  pending_pay:     ['in_service', 'cancelled'],
  in_service:      ['pending_confirm', 'disputed'],
  pending_confirm: ['completed', 'disputed', 'in_service'],
  completed:       ['settled', 'disputed'],
  disputed:        ['refunded', 'completed', 'in_service'],
};

/** 终态：不会再自己变化，也不该再给用户任何"下一步"按钮。 */
export const TERMINAL_STATUS = ['settled', 'refunded', 'cancelled'];

export const ORDER_TYPE_LABEL = { question: '悬赏问答', booking: '预约服务' };

export const SERVICE_MODE_LABEL = { im: '图文沟通', voice: '语音', video: '视频', screen: '远程协助' };

/** 支付通道的中文名。名字以后端 payment 包的 channelLabel 为准，这里只做兜底。 */
export const PAY_CHANNEL_LABEL = {
  mock: '模拟支付', offline: '线下转账', wechat: '微信支付', alipay: '支付宝',
};

/** 问题状态文案（"我的悬赏"用，与订单状态是两套东西）。 */
export const QUESTION_STATUS_LABEL = {
  open: '待响应', matched: '已接单', closed: '已解决',
  expired: '已过期', cancelled: '已取消',
};

/* ------------------------------------------------------------ 金额 -- */

/**
 * 分 → 「¥1,280.00」。
 *
 * 接口通篇用「分」传金额（int64），换算只在前端做一次，避免出现
 * "有的地方是元、有的地方是分"这种对不上账的问题。
 */
export function fmtCents(cents) {
  const n = Number(cents || 0);
  const sign = n < 0 ? '-' : '';
  const abs = Math.abs(n);
  const yuan = Math.floor(abs / 100);
  const dec = String(abs % 100).padStart(2, '0');
  return `${sign}¥${String(yuan).replace(/\B(?=(\d{3})+(?!\d))/g, ',')}.${dec}`;
}

/* ---------------------------------------------------------- 倒计时 -- */

/**
 * 距某个截止时间还有多久。
 *
 * 由前端按绝对时间算剩余，而不是让服务端返回"剩余 N 秒"：服务端算好的
 * 秒数在网络上就开始过时，用户看到的分秒会和真实截止时间差出一截。
 */
export function remainText(iso) {
  if (!iso) return '';
  const ms = new Date(iso).getTime() - Date.now();
  if (!Number.isFinite(ms)) return '';
  if (ms <= 0) return '已超时';
  const sec = Math.floor(ms / 1000);
  if (sec >= 3600) {
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    return `剩余 ${h} 小时 ${m} 分`;
  }
  if (sec >= 60) return `剩余 ${Math.floor(sec / 60)} 分 ${sec % 60} 秒`;
  return `剩余 ${sec} 秒`;
}

/* ------------------------------------------------------------ 展示 -- */

export function statusChipHTML(status, extra = '') {
  const meta = ORDER_STATUS[status] || { label: status || '未知', tone: 'muted' };
  return `<span class="status-chip is-${esc(meta.tone)} ${esc(extra)}">${esc(meta.label)}</span>`;
}

export function statusDesc(status) {
  return (ORDER_STATUS[status] || {}).desc || '';
}

/** 一条订单上"这个人现在能做什么"。role: user=下单人，expert=服务专家。 */
export function orderActions(order, role) {
  const st = order && order.status;
  const acts = [];
  if (!st) return acts;

  if (role === 'expert') {
    // 专家侧的动作都发生在服务中之后，待接单阶段属于大厅，不在这里
    if (st === 'in_service') acts.push('session', 'finish');
    if (order.review && !order.review.reply) acts.push('reply');
    // 只有差评才值得申诉，好评去申诉既没意义也会被后台驳回
    if (order.review && order.review.score <= 3 && !order.review.appealed) acts.push('appeal');
    return acts;
  }

  if (st === 'pending_pay') acts.push('pay', 'cancel');
  if (st === 'pending_accept') acts.push('cancel');
  if (st === 'pending_confirm') acts.push('confirm', 'dispute');
  if (st === 'in_service') acts.push('dispute');
  if ((st === 'completed' || st === 'settled') && !order.reviewed) acts.push('review');
  return acts;
}

export const ACTION_LABEL = {
  pay: '去支付', cancel: '取消订单', confirm: '确认完成', dispute: '申请退款 / 争议',
  review: '评价服务', session: '开始会话', finish: '标记完成',
  reply: '回复评价', appeal: '申诉差评',
};
