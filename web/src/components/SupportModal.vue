<template>
  <Teleport to="body">
    <div v-if="support.show" class="support-backdrop" @click.self="support.dismiss()">
      <section class="support-card" role="dialog" aria-modal="true" aria-label="赞赏支持">
        <button class="icon-btn support-close" aria-label="关闭" @click="support.dismiss()"><AppIcon name="close" :size="18" /></button>
        <div class="support-icon"><AppIcon name="heart" :size="22" /></div>
        <h3>请作者喝杯咖啡</h3>
        <p class="support-desc">应用免费无广告、数据保存在本机。若对你有帮助，欢迎赞赏——<b>金额随意，1 元也是心意</b>。</p>
        <img class="support-qr" :src="donateQr" alt="微信赞赏码" />
        <div class="support-qr-tip">微信扫码赞赏</div>
        <p v-if="support.errorText" class="support-error">{{ support.errorText }}</p>
        <div class="support-actions">
          <button class="btn btn-outline" :disabled="support.sending" @click="support.dismiss()">暂不支持</button>
          <button class="btn btn-primary" :disabled="support.sending" @click="openAmount">已支持</button>
        </div>
      </section>
    </div>

    <div v-if="amountOpen" class="support-backdrop support-backdrop-top" @click.self="closeAmount">
      <section class="support-card support-amount" role="dialog" aria-modal="true" aria-label="赞赏金额">
        <div class="sheet-head"><h2>赞赏金额</h2><button class="icon-btn" aria-label="关闭" @click="closeAmount"><AppIcon name="close" :size="18" /></button></div>
        <div class="field"><label>金额（元）</label><input v-model.number="amount" type="number" min="0" step="1" class="input" placeholder="例如：6" /></div>
        <p class="tiny muted">填写自愿赞赏的金额，点击确定后本版本隐藏横幅。</p>
        <div class="sheet-foot"><button class="btn btn-outline btn-block" :disabled="support.sending" @click="closeAmount">取消</button><button class="btn btn-primary btn-block" :disabled="support.sending || amount < 0" @click="confirmAmount">{{ support.sending ? '提交中…' : '确定' }}</button></div>
      </section>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import AppIcon from './AppIcon.vue'
import donateQr from '../assets/donate-wechat.png'
import { useSupportStore } from '../stores/support'
import { useUIStore } from '../stores/ui'

const support = useSupportStore()
const ui = useUIStore()
const amountOpen = ref(false)
const amount = ref(6)

function openAmount() {
  support.markPermanentSuppressed()
  amount.value = 6
  amountOpen.value = true
}
function closeAmount() { if (!support.sending) amountOpen.value = false }
async function confirmAmount() {
  const ok = await support.markSupported(Number(amount.value) || 0)
  if (ok) { amountOpen.value = false; ui.showToast('感谢支持 ❤') }
}
</script>

<style scoped>
.support-backdrop { position: fixed; inset: 0; z-index: 130; background: rgba(0, 0, 0, .45); display: flex; align-items: center; justify-content: center; padding: 16px; }
.support-backdrop-top { z-index: 140; }
.support-card { position: relative; width: 100%; max-width: 340px; max-height: 92dvh; overflow-y: auto; background: var(--mc-surface); border: 1px solid var(--mc-border); border-radius: 20px; box-shadow: 0 24px 60px rgba(20,28,24,.26); padding: 20px 18px 16px; text-align: center; }
.support-amount { text-align: left; padding: 16px; }
.support-close { position: absolute; top: 8px; right: 8px; }
.support-icon { width: 46px; height: 46px; margin: 0 auto 8px; border-radius: 15px; display: grid; place-items: center; background: var(--mc-primary-soft); color: var(--mc-primary); }
.support-card h3 { font-size: 16px; font-weight: 800; }
.support-desc { margin-top: 6px; font-size: 12px; line-height: 1.65; color: var(--mc-text-muted); }
.support-qr { display: block; width: 150px; max-width: 100%; height: auto; margin: 10px auto 4px; padding: 5px; background: #fff; border: 1px solid var(--mc-border); border-radius: 10px; }
.support-qr-tip { font-size: 11px; color: var(--mc-text-muted); }
.support-error { margin-top: 8px; font-size: 12px; color: var(--mc-danger); }
.support-actions { display: flex; gap: 8px; margin-top: 14px; }
.support-actions .btn { flex: 1 1 0; min-width: 0; }
</style>
