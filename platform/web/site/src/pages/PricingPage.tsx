import Pricing from '../components/Pricing'
import FAQ from '../components/FAQ'

export default function PricingPage() {
  return (
    <main className="overflow-x-hidden pt-28">
      <Pricing standalone />
      <FAQ compact />
    </main>
  )
}
