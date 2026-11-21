import { createApp } from 'vue'
import { createPinia } from 'pinia'
import router from './router'
import App from './App.vue'
import { useThemeStore } from './stores/theme'
import { applyMedicineDict } from './utils/medicineMeta'
import './style.css'
import './styles/themes.css'
import './styles/auth.css'

const app = createApp(App)
app.use(createPinia())
useThemeStore().init()
app.use(router)
app.mount('#app')

// 启动时拉一次公开配置里的药品字典（剂型/规格/单位/包装规格），
// 让管理员在设置里维护的候选值在各页面下拉直接生效。
fetch('/api/configs/public')
  .then((r) => r.json())
  .then((j) => {
    const d = j?.data || {}
    applyMedicineDict({
      form: String(d.medcare_form_options || ''),
      spec: String(d.medcare_spec_options || ''),
      unit: String(d.medcare_unit_options || ''),
      packageSpec: String(d.medcare_package_spec_options || ''),
    })
  })
  .catch(() => { /* 拿不到就用内置默认值 */ })
