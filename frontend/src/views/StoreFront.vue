<template>
  <div class="min-h-screen bg-tg-secondary-bg p-4 flex flex-col gap-4 pb-20">
    <!-- Header Toko -->
    <div class="bg-tg-bg p-4 rounded-xl shadow-sm text-center border border-black/5">
      <h1 class="text-xl font-bold text-tg-text">🛒 Gopher Store</h1>
      <p class="text-xs text-tg-hint mt-1">Platform Belanja Webassembly Golang + Vue 3</p>
    </div>

    <!-- Info User Telegram -->
    <div v-if="user" class="bg-tg-bg p-3 rounded-xl shadow-sm flex items-center gap-3 border border-black/5">
      <div class="w-10 h-10 rounded-full bg-tg-button text-tg-button-text flex items-center justify-center font-bold text-sm">
        {{ user.first_name.slice(0, 2).toUpperCase() }}
      </div>
      <div>
        <p class="text-sm font-semibold text-tg-text">Halo, {{ user.first_name }} {{ user.last_name || '' }}</p>
        <p class="text-xs text-tg-hint">ID: {{ user.id }} <span v-if="user.username">(@{{ user.username }})</span></p>
      </div>
    </div>

    <!-- State Loading -->
    <div v-if="loading" class="text-center text-tg-hint py-8">
      Menghubungkan ke Edge Cloudflare Worker...
    </div>

    <!-- Daftar Produk -->
    <div v-else class="grid grid-cols-1 gap-4">
      <div 
        v-for="product in products" 
        :key="product.id" 
        class="bg-tg-bg p-4 rounded-xl flex items-center justify-between shadow-sm border border-black/5"
      >
        <div class="flex items-center gap-3">
          <img :src="product.image_url" class="w-16 h-16 rounded-lg bg-gray-100 object-cover" />
          <div>
            <h2 class="font-bold text-tg-text text-sm">{{ product.name }}</h2>
            <p class="text-xs text-tg-hint line-clamp-2 mt-0.5">{{ product.description }}</p>
            <p class="text-sm font-semibold text-tg-link mt-1">Rp {{ product.price.toLocaleString('id-ID') }}</p>
          </div>
        </div>
        <button 
          @click="checkout(product)" 
          class="bg-tg-button text-tg-button-text px-4 py-2 rounded-lg text-xs font-semibold active:scale-95 transition-transform"
        >
          Beli
        </button>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, onMounted } from 'vue'
import { usePopup, useHapticFeedback } from 'vue-tg'

interface Product {
  id: number
  name: string
  price: number
  description: string
  image_url: string
}

interface TelegramUser {
  id: number
  first_name: string
  last_name?: string
  username?: string
}

const products = ref<Product[]>([])
const loading = ref<boolean>(true)
const user = ref<TelegramUser | null>(null)

// GANTI DENGAN URL WORKER ANDA YANG AKTIF
const backendURL = "https://telegram-store-backend.YOURSUBDOMAIN.workers.dev" 

const popup = usePopup()
const haptic = useHapticFeedback()

const initTelegramUser = () => {
  const telegramWebApp = (window as any).Telegram?.WebApp
  if (telegramWebApp && telegramWebApp.initDataUnsafe?.user) {
    user.value = telegramWebApp.initDataUnsafe.user
    telegramWebApp.ready()
    telegramWebApp.expand()
  } else {
    // Mode fallback jika diakses di luar telegram untuk pengujian lokal
    user.value = {
      id: 99999999,
      first_name: "Tamu",
      last_name: "Lokal",
      username: "tamu_lokal"
    }
  }
}

const fetchProducts = async () => {
  try {
    const response = await fetch(`${backendURL}/api/products`)
    if (!response.ok) throw new Error("Gagal mengambil data dari server")
    products.value = await response.json()
  } catch (error) {
    console.error("Gagal memuat produk:", error)
    popup.showAlert("Sistem gagal memuat katalog. Pastikan koneksi internet stabil.")
  } finally {
    loading.value = false
  }
}

const checkout = async (product: Product) => {
  haptic.impactOccurred('medium')
  
  const telegramWebApp = (window as any).Telegram?.WebApp
  const initData = telegramWebApp ? telegramWebApp.initData : ""

  if (!initData) {
    popup.showAlert("Silakan buka aplikasi ini langsung di dalam aplikasi Telegram.")
    return
  }

  try {
    const response = await fetch(`${backendURL}/api/orders`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Telegram-Init-Data': initData
      },
      body: JSON.stringify({ product_id: product.id })
    })

    if (!response.ok) {
      const errorData = await response.json()
      throw new Error(errorData.error || "Gagal membuat pesanan")
    }

    haptic.notificationOccurred('success')
    popup.showAlert(`Pesanan untuk "${product.name}" berhasil dicatat!`)
  } catch (error: any) {
    haptic.notificationOccurred('error')
    popup.showAlert(`Gagal membuat pesanan: ${error.message}`)
  }
}

onMounted(() => {
  initTelegramUser()
  fetchProducts()
})
</script>
