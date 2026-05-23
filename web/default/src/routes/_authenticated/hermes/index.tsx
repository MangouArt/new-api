import { createFileRoute } from '@tanstack/react-router'
import { Hermes } from '@/features/hermes'

export const Route = createFileRoute('/_authenticated/hermes/')({
  component: Hermes,
})
