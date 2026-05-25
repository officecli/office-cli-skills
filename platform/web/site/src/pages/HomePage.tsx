import Hero from '../components/Hero'
import Features from '../components/Features'
import InstallTabs from '../components/InstallTabs'
import Workflow from '../components/Workflow'
import CLIShowcase from '../components/CLIShowcase'
import UseCases from '../components/UseCases'
import Roadmap from '../components/Roadmap'
import Pricing from '../components/Pricing'
import FAQ from '../components/FAQ'
import Contact from '../components/Contact'
import VibeOfficing from '../components/VibeOfficing'

export default function HomePage() {
  return (
    <main className="overflow-x-hidden">
      <Hero />
      <VibeOfficing />
      <Features />
      <CLIShowcase />
      <Workflow />
      <UseCases />
      <InstallTabs />
      <Roadmap />
      <Pricing />
      <FAQ />
      <Contact />
    </main>
  )
}
