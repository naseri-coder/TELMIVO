import {createRoot} from 'react-dom/client';
import {BrowserRouter} from 'react-router-dom';
import '@fontsource/vazirmatn/400.css';
import '@fontsource/vazirmatn/600.css';
import './styles/app.css';
import App from './app/App';
createRoot(document.getElementById('root')!).render(<BrowserRouter><App/></BrowserRouter>);
