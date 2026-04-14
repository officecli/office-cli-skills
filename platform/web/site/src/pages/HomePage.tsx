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

export default function HomePage() {
  return (
    <>
      <Hero />
      <Features />
      <InstallTabs />
      <Workflow />
      <CLIShowcase />
      <UseCases />
      <Roadmap />
      <Pricing />
      <FAQ />
      <Contact />
    </>
  )
}
