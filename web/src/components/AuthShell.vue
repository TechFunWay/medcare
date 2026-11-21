<template>
  <div class="auth-page">
    <div class="auth-layout">
      <aside class="auth-hero">
        <div class="auth-brand">
          <div class="auth-mark">药</div>
          <div>
            <div class="auth-brand-name">用药管理</div>
            <div class="auth-brand-sub">家庭用药本</div>
          </div>
        </div>
        <h2 class="auth-hero-title">把每一次吃药、打针、输液<br />都记清楚，家人可以一起管</h2>
        <ul class="auth-points">
          <li v-for="p in points" :key="p">{{ p }}</li>
        </ul>
        <p class="auth-hero-note">本应用只帮助记录和提醒，不提供诊断或用药建议。请遵医嘱。</p>
      </aside>

      <div class="auth-card card">
        <div class="auth-brand auth-brand--compact">
          <div class="auth-mark">药</div>
          <div>
            <div class="auth-brand-name">用药管理</div>
            <div class="auth-brand-sub">家庭用药本</div>
          </div>
        </div>
        <slot />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ points?: string[] }>(), {
  points: () => ['药品批次与库存、用药计划、到点提醒与打卡', '管理员、监护人、成员三级权限，一起照护不越权', '邮件、钉钉、飞书多渠道提醒，处方单 AI 识别'],
})
</script>
