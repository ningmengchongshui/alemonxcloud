import { ImageSourceEditor } from '@/components/ImageSourceEditor'
import { PlanEditor } from '@/components/PlanEditor'

// Composes independent catalog forms without sharing their dialog or input state.
export function CatalogEditor() {
  return (
    <div className="flex flex-wrap gap-2">
      <ImageSourceEditor />
      <PlanEditor />
    </div>
  )
}
