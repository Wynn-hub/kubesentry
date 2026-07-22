import { ref } from 'vue'

export function useLatestRequest() {
  const loading = ref(false)
  let latestRequestID = 0

  async function run<T>(request: () => Promise<T>, commit: (value: T) => void): Promise<boolean> {
    const requestID = ++latestRequestID
    loading.value = true
    try {
      const value = await request()
      if (requestID !== latestRequestID) return false
      commit(value)
      return true
    } catch (error) {
      if (requestID !== latestRequestID) return false
      throw error
    } finally {
      if (requestID === latestRequestID) loading.value = false
    }
  }

  return { loading, run }
}
