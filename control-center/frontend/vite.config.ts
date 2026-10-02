import {defineConfig} from 'vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'
import {readFileSync} from 'node:fs'

// package.json is the single source of the version shown in the interface.
const {version} = JSON.parse(readFileSync(new URL('./package.json', import.meta.url), 'utf-8'))

// https://vitejs.dev/config/
export default defineConfig({
	plugins: [tailwindcss(), svelte()],
	define: {
		__APP_VERSION__: JSON.stringify(version),
	},
	test: {
		environment: 'node',
	},
})
